package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/beevik/etree"
	"github.com/mpetavy/common"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// windows: koq -h "c:\Program Files\HandBrake\HandBrakeCLI.exe" -o "z:\Media\Videos\King of Queens" -d g:

var (
	minLength    *time.Duration
	preset       *string
	handbrake    *string
	device       *string
	format       *string
	videoEncoder *string
	audioEncoder *string
	language     *string
	output       *string
	title        *string
)

const (
	windowsEjectScript = "Set oWMP = CreateObject(\"WMPlayer.OCX.7\" )\nSet colCDROMs = oWMP.cdromCollection\n\nif colCDROMs.Count >= 1 then\n        For i = 0 to colCDROMs.Count - 1\n\t\t\t\tif lcase(colCDROMs.Item(i).driveSpecifier) = lcase(wscript.Arguments.Item(0)) then\n\t\t\t\t\tcolCDROMs.Item(i).Eject\n\t\t\t\tend if\n        Next\nEnd If\n"
)

func init() {
	common.Init(false, "1.0.0", "", "2021", "Rescues my KoQ discs (and others...)", "mpetavy", fmt.Sprintf("https://github.com/mpetavy/%s", common.Title()), common.APACHE, nil, nil, run, 0)

	minLength = flag.Duration("min", time.Minute*10, "minimum duration to consider as valid track")
	preset = flag.String("p", "Fast 720p30", "device to read the DVD content")
	handbrake = flag.String("h", "HandBrakeCLI", "path to Handbrake CLI executable")
	device = flag.String("d", "/dev/dvd", "device to read the DVD content")
	format = flag.String("f", "av_mp4", "Handbrake video format")
	videoEncoder = flag.String("v", "nvenc_h264", "Handbrake video encoder")
	audioEncoder = flag.String("a", "copy:ac3", "Handbrake audio encoder")
	language = flag.String("l", "de", "Handbrake language")
	output = flag.String("o", ".", "Output directory")
	title = flag.String("t", "", "Title to use")
}

func readMetadata() (*etree.Document,error) {
	ba := []byte{}

	if common.IsWindowsOS() {
		var err error

		cmd := exec.Command("cmd.exe","/k","dir " + *device)
		ba, err = cmd.Output()
		if common.Error(err) {
			return nil,err
		}

		scanner := bufio.NewScanner(strings.NewReader(string(ba)))
		if scanner.Scan() {
			line := scanner.Text()

			p := strings.LastIndex(line," ")
			if p != -1 {
				*title = line[p+1:]
			}
		}

		cmd = exec.Command("wsl","--","mkdir","-p","/tmp/mnt;sudo","mount","-t","drvfs","g:","/tmp/mnt;","lsdvd","-Ox","-a","-v","/tmp/mnt;","sudo","umount","/tmp/mnt;","rm","-rf","/tmp/mnt")
		ba, err = cmd.Output()
		if common.Error(err) {
			return nil,err
		}
	} else {
		var err error

		cmd := exec.Command("lsdvd", "-Ox", "-a", "-v")
		ba, err = cmd.Output()
		if common.Error(err) {
			return nil,err
		}
	}

	doc := etree.NewDocument()
	err := doc.ReadFromBytes(ba)

	return doc,err
}

func eject() error {
	if common.IsWindowsOS() {
		f,err := common.CreateTempFile()
		if common.Error(err) {
			return err
		}

		filename := f.Name() + ".vbs"

		err = ioutil.WriteFile(filename,[]byte(windowsEjectScript),common.DefaultFileMode)
		if common.Error(err) {
			return err
		}

		defer func() {
			common.DebugError(common.FileDelete(filename))
		}()

		cmd := exec.Command("cscript",filename,*device)
		err = cmd.Run()
		if common.Error(err) {
			return err
		}
	} else {
		cmd := exec.Command("eject", *device)
		err := cmd.Run()
		if common.Error(err) {
			return err
		}
	}

	return nil
}

func run() error {
	b, _ := common.FileExists(*output)
	if !b {
		return &common.ErrFileNotFound{
			FileName: *output,
		}
	}

	b, _ = common.IsDirectory(*output)
	if !b {
		return fmt.Errorf("%s is not a directory", *output)
	}

	doc,err := readMetadata()
	if common.Error(err) {
		return err
	}

	rootElem := doc.SelectElement("lsdvd")

	if *title == "" {
		titleElem := rootElem.FindElement("//lsdvd/title")
		if titleElem == nil {
			return fmt.Errorf("cannot find title element")
		}
		if titleElem.Text() == "unknown" {
			return fmt.Errorf("found DVD title is 'unknown', please provide title")
		}

		*title = titleElem.Text()
	}

	allStart := time.Now()
	index := 0

	common.Info("Title: %s",*title)
	common.Info("")

	for _, trackElem := range rootElem.SelectElements("track") {
		indexElem := trackElem.FindElement("ix")
		lengthElem := trackElem.FindElement("length")

		secs, err := strconv.ParseFloat(lengthElem.Text(), 64)
		if common.Error(err) {
			return err
		}

		secsDuration := time.Second * time.Duration(secs)

		common.Info("Track %s: %v", indexElem.Text(), secsDuration)

		if secsDuration < *minLength {
			common.Info("track too short  -> skip!")

			continue
		}

		ext := *format
		p := strings.LastIndex(*format, "_")
		if p != -1 {
			ext = ext[p+1:]
		}

		ss := strings.Split(*title, "_")

		var sb strings.Builder

		for _, s := range ss {
			if sb.Len() > 0 {
				sb.WriteString(" ")
			}

			sb.WriteString(common.Capitalize(strings.ToLower(s)))
		}

		*title = sb.String()

		index++
		filename := common.CleanPath(filepath.Join(*output, *title, *title+" - "+fmt.Sprintf("%02d", index)+"."+ext))

		b, _ = common.FileExists(filename)

		if b {
			common.Info("target file %s already exists -> skip!", filename)

			continue
		}

		err = os.MkdirAll(filepath.Dir(filename), common.DefaultDirMode)
		if common.Error(err) {
			return err
		}

		common.Info("Start: %v", time.Now().Format(common.DateTimeMask))

		cmd := exec.Command(*handbrake,
			"--title", indexElem.Text(),
			"--preset", *preset,
			"--input", *device,
			"--output", filename,
			"--format", *format,
			"--optimize",
			"--keep-display-aspect",
			"--comb-detect",
			"--decomb",
			"--encoder="+*videoEncoder,
			"--audio-lang-list="+*language,
			"--aencoder="+*audioEncoder,
			"--loose-crop",
			"--subtitle", "scan",
			"--subtitle-forced",
			"--subtitle-burned",
			"--native-language="+*language)

		common.Info("Execute: %s",common.CmdToString(cmd))

		start := time.Now()

		err = cmd.Run()
		if common.Error(err) {
			return err
		}

		common.Info("End: %v", time.Now().Format(common.DateTimeMask))

		common.Info("Time needed: %v", time.Since(start))
		common.Info("")
	}

	common.Info("Total time needed: %v\n\n", time.Since(allStart))

	err = eject()
	if common.Error(err) {
		return err
	}

	return nil
}

func main() {
	defer common.Done()

	common.Run(nil)
}
