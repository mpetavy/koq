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
	input        *string
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
	common.Init(false, "1.0.0", "", "", "2021", "Rescues my KoQ discs (and others...)", "mpetavy", fmt.Sprintf("https://github.com/mpetavy/%s", common.Title()), common.APACHE, nil, nil, nil, run, 0)

	minLength = flag.Duration("min", time.Minute*10, "minimum duration to consider as valid track")
	preset = flag.String("p", "Fast 720p30", "device to read the DVD content")
	handbrake = flag.String("b", "HandBrakeCLI", "path to Handbrake CLI executable")

	var drive string

	drives := []string{"/dev/dvd", "/dev/sr0", "dev(cdrom"}

	for _, d := range drives {
		if common.FileExists(d) {
			drive = d
			break
		}
	}
	input = flag.String("i", drive, "Input device to read the DVD content")
	format = flag.String("f", "av_mp4", "Handbrake video format")
	videoEncoder = flag.String("v", "nvenc_h264", "Handbrake video encoder")
	audioEncoder = flag.String("a", "copy:ac3", "Handbrake audio encoder")
	language = flag.String("l", "de", "Handbrake language")

	ud, _ := os.UserHomeDir()
	ud = fmt.Sprintf("%s/Videos", ud)

	var o string
	if common.IsWindowsOS() || ud == "" || !common.FileExists(ud) {
		o = "."
	} else {
		o = ud
	}
	output = flag.String("o", o, "Output directory")

	title = flag.String("t", "", "Title to use")
}

func readMetadata() (string, *etree.Document, error) {
	ba := []byte{}
	dvdTitle := *title

	if common.IsWindowsOS() {
		var err error

		cmd := exec.Command("cmd.exe", "/k", "dir "+*input)
		ba, err = cmd.Output()
		if common.Error(err) {
			return dvdTitle, nil, err
		}

		if dvdTitle == "" {
			scanner := bufio.NewScanner(strings.NewReader(string(ba)))
			if scanner.Scan() {
				line := scanner.Text()

				p := strings.LastIndex(line, " ")
				if p != -1 {
					dvdTitle = line[p+1:]
				}
			}
		}

		cmd = exec.Command("wsl", "--", "mkdir", "-p", "/tmp/mnt;sudo", "mount", "-t", "drvfs", "g:", "/tmp/mnt;", "lsdvd", "-Ox", "-a", "-v", "/tmp/mnt;", "sudo", "umount", "/tmp/mnt;", "rm", "-rf", "/tmp/mnt")

		common.Info("Execute: %s", common.CmdToString(cmd))

		ba, err = common.WatchdogCmd(cmd, time.Second*3)
		if common.Error(err) {
			return dvdTitle, nil, err
		}
	} else {
		var err error

		cmd := exec.Command("lsdvd", "-Ox", "-a", "-v", *input)

		common.Info("Execute: %s", common.CmdToString(cmd))

		ba, err = cmd.Output()
		if common.Error(err) {
			return dvdTitle, nil, err
		}
	}

	doc := etree.NewDocument()
	doc.ReadSettings.Permissive = true

	err := doc.ReadFromBytes(ba)
	if common.Error(err) {
		return dvdTitle, nil, err
	}

	rootElem := doc.SelectElement("lsdvd")
	if rootElem == nil {
		return dvdTitle, nil, fmt.Errorf("cannot find root element 'lsdvd'")
	}

	if dvdTitle == "" {
		titleElem := rootElem.FindElement("//lsdvd/title")
		if titleElem == nil {
			return dvdTitle, nil, fmt.Errorf("cannot find title element")
		}
		if titleElem.Text() == "unknown" {
			return dvdTitle, nil, fmt.Errorf("found DVD title is 'unknown', please provide title")
		}

		dvdTitle = titleElem.Text()
	}

	ss := strings.Split(dvdTitle, "_")

	var sb strings.Builder

	for _, s := range ss {
		if sb.Len() > 0 {
			sb.WriteString(" ")
		}

		sb.WriteString(common.Capitalize(strings.ToLower(s)))
	}

	dvdTitle = sb.String()

	return dvdTitle, doc, nil
}

func eject() error {
	if common.IsWindowsOS() {
		f, err := common.CreateTempFile()
		if common.Error(err) {
			return err
		}

		filename := f.Name() + ".vbs"

		err = ioutil.WriteFile(filename, []byte(windowsEjectScript), common.DefaultFileMode)
		if common.Error(err) {
			return err
		}

		defer func() {
			common.DebugError(common.FileDelete(filename))
		}()

		cmd := exec.Command("cscript", filename, *input)
		err = cmd.Run()
		if common.Error(err) {
			return err
		}
	} else {
		cmd := exec.Command("eject", *input)
		err := cmd.Run()
		if common.Error(err) {
			return err
		}
	}

	return nil
}

func run() error {
	b := common.FileExists(*output)
	if !b {
		return &common.ErrFileNotFound{
			FileName: *output,
		}
	}

	if !common.IsDirectory(*output) {
		return fmt.Errorf("%s is not a directory", *output)
	}

	dvdTitle, doc, err := readMetadata()
	if common.Error(err) {
		return err
	}

	rootElem := doc.SelectElement("lsdvd")
	allStart := time.Now()

	if *title == "" {
		titleElem := rootElem.FindElement("//lsdvd/title")
		if titleElem == nil {
			return fmt.Errorf("cannot find title element")
		}

		*title = titleElem.Text()
	}

	index := 0

	common.Info("Found title: %s", dvdTitle)
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

		index++

		filename := common.CleanPath(filepath.Join(*output, dvdTitle, dvdTitle+" - "+fmt.Sprintf("%02d", index)+"."+ext))
		ss := strings.Split(*title, "_")

		var sb strings.Builder

		for _, s := range ss {
			if sb.Len() > 0 {
				sb.WriteString(" ")
			}

			sb.WriteString(common.Capitalize(strings.ToLower(s)))
		}

		*title = sb.String()

		b = common.FileExists(filename)

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
			"--input", *input,
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

		common.Info("Execute: %s", common.CmdToString(cmd))

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

	common.Run([]string{"i"})
}
