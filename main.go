package main

import (
	"bufio"
	"embed"
	"flag"
	"fmt"
	"github.com/beevik/etree"
	"github.com/mpetavy/common"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// windows: koq -h "c:\Program Files\HandBrake\HandBrakeCLI.exe" -o "z:\Media\Videos\King of Queens" -d g:

var (
	dvd          string
	minLength    *time.Duration
	preset       *string
	handbrake    *string
	input        common.MultiValueFlag
	format       *string
	videoEncoder *string
	audioEncoder *string
	language     *string
	output       *string
	startTime    *string
	stopTime     *string
	title        common.MultiValueFlag
)

const (
	windowsEjectScript = "Set oWMP = CreateObject(\"WMPlayer.OCX.7\" )\nSet colCDROMs = oWMP.cdromCollection\n\nif colCDROMs.Count >= 1 then\n        For i = 0 to colCDROMs.Count - 1\n\t\t\t\tif lcase(colCDROMs.Item(i).driveSpecifier) = lcase(wscript.Arguments.Item(0)) then\n\t\t\t\t\tcolCDROMs.Item(i).Eject\n\t\t\t\tend if\n        Next\nEnd If\n"
)

//go:embed go.mod
var resources embed.FS

func init() {
	common.Init("", "", "", "", "Rescues my KoQ discs (and others...)", "", "", "", &resources, nil, nil, run, 0)

	minLength = flag.Duration("min", time.Minute*10, "minimum duration to consider as valid track")
	preset = flag.String("p", "Fast 1080p30", "device to read the DVD content")
	handbrake = flag.String("b", "HandBrakeCLI", "path to Handbrake CLI executable")

	for _, d := range []string{"/dev/dvd", "/dev/sr0", "dev/cdrom"} {
		if common.FileExists(d) {
			dvd = d
			break
		}
	}

	flag.Var(&input, "i", "Input files to read")
	format = flag.String("x", "av_mp4", "Handbrake video format")
	videoEncoder = flag.String("video", "nvenc_h264", "Handbrake video encoder")
	audioEncoder = flag.String("audio", "copy", "Handbrake audio encoder")
	language = flag.String("l", "ger,eng", "Handbrake language")
	startTime = flag.String("start", "", "Handbrake start-at duration in secs")
	stopTime = flag.String("stop", "", "Handbrake stop-at duration in secs")

	ud, _ := os.UserHomeDir()
	ud = fmt.Sprintf("%s/Videos", ud)

	var o string
	if common.IsWindows() || ud == "" || !common.FileExists(ud) {
		o = "."
	} else {
		o = ud
	}
	output = flag.String("o", o, "Output directory")
	flag.Var(&title, "t", "Input files to read")

	common.Events.AddListener(common.EventFlags{}, func(event common.Event) {
		if len(input) == 0 && dvd != "" {
			input = []string{dvd}
		}
	})
}

func readMetadata(file string) (string, *etree.Document, error) {
	ba := []byte{}

	var dvdTitle string

	if common.IsWindows() {
		var err error

		cmd := exec.Command("cmd.exe", "/k", "dir "+file)
		ba, err = cmd.Output()
		if common.Error(err) {
			return "", nil, err
		}

		scanner := bufio.NewScanner(strings.NewReader(string(ba)))
		if scanner.Scan() {
			line := scanner.Text()

			p := strings.LastIndex(line, " ")
			if p != -1 {
				dvdTitle = line[p+1:]
			}
		}

		cmd = exec.Command("wsl", "--", "mkdir", "-p", "/tmp/mnt;sudo", "mount", "-t", "drvfs", "g:", "/tmp/mnt;", "lsdvd", "-Ox", "-a", "-v", "/tmp/mnt;", "sudo", "umount", "/tmp/mnt;", "rm", "-rf", "/tmp/mnt")

		common.Info("Execute: %s", common.CmdToString(cmd))

		ba, err = common.NewWatchdogCmd(cmd, time.Second*3)
		if common.Error(err) {
			return "", nil, err
		}
	} else {
		var err error

		cmd := exec.Command("lsdvd", "-Ox", "-a", "-v", file)

		common.Info("Execute: %s", common.CmdToString(cmd))

		ba, err = cmd.Output()
		if common.Error(err) {
			return "", nil, err
		}
	}

	sb := strings.Builder{}

	scanner := bufio.NewScanner(strings.NewReader(string(ba)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "<langcode>") {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	doc := etree.NewDocument()
	doc.ReadSettings.Permissive = true

	cleanedXml := sb.String()

	err := doc.ReadFromBytes([]byte(cleanedXml))
	if common.Error(err) {
		return "", nil, err
	}

	rootElem := doc.SelectElement("lsdvd")
	if rootElem == nil {
		return "", nil, fmt.Errorf("cannot find root element 'lsdvd'")
	}

	if dvdTitle == "" {
		titleElem := rootElem.FindElement("//lsdvd/title")
		if titleElem == nil {
			return "", nil, fmt.Errorf("cannot find title element")
		}
		if titleElem.Text() == "unknown" {
			return "", nil, fmt.Errorf("found DVD title is 'unknown', please provide title")
		}

		dvdTitle = titleElem.Text()
	}

	ss := strings.Split(dvdTitle, "_")

	sb.Reset()

	for _, s := range ss {
		if sb.Len() > 0 {
			sb.WriteString(" ")
		}

		sb.WriteString(common.Capitalize(strings.ToLower(s)))
	}

	dvdTitle = sb.String()

	return dvdTitle, doc, nil
}

func eject(file string) error {
	if common.IsWindows() {
		f, err := common.CreateTempFile()
		if common.Error(err) {
			return err
		}

		filename := f.Name() + ".vbs"

		err = os.WriteFile(filename, []byte(windowsEjectScript), common.DefaultFileMode)
		if common.Error(err) {
			return err
		}

		defer func() {
			common.DebugError(common.FileDelete(filename))
		}()

		cmd := exec.Command("cscript", filename, file)
		err = cmd.Run()
		if common.Error(err) {
			return err
		}
	} else {
		cmd := exec.Command("eject", file)
		err := cmd.Run()
		if common.Error(err) {
			return err
		}
	}

	return nil
}

func encode(source string, title string, dest string) error {
	common.Info("Encode: %s -> %s", source, dest)

	common.Info("Start: %v", time.Now().Format(common.DateTimeMask))

	args := []string{
		"--preset", *preset,
		"--input", source,
		"--output", dest,
		"--format", *format,
		"--optimize",
		"--keep-display-aspect",
		"--comb-detect",
		"--decomb",
		"--encoder=" + *videoEncoder,
		"--audio-lang-list=" + *language,
		"--aencoder=" + *audioEncoder,
		"--loose-crop",
		"--subtitle", "scan",
		"--subtitle-forced",
		"--subtitle-burned",
		"--native-language=" + *language,
	}

	if title != "" {
		args = append(args, "--title", title)
	}

	if *startTime != "" {
		args = append(args, "--start-at", "duration:"+*startTime)
	}

	if *stopTime != "" {
		args = append(args, "--stop-at", "duration:"+*stopTime)
	}

	cmd := exec.Command(*handbrake, args...)

	common.Info("Execute: %s", common.CmdToString(cmd))

	start := time.Now()

	err := cmd.Run()
	if common.Error(err) {
		return err
	}

	common.Info("End: %v", time.Now().Format(common.DateTimeMask))

	common.Info("Time needed: %v", time.Since(start))

	return err
}

func process(source string, title string) error {
	b := common.FileExists(*output)
	if !b {
		return &common.ErrFileNotFound{
			FileName: *output,
		}
	}

	if !common.IsDirectory(*output) {
		return fmt.Errorf("%s is not a directory", *output)
	}

	stat, err := os.Stat(source)
	if common.Error(err) {
		return err
	}

	if stat.Mode().IsRegular() {
		if title == "" {
			return fmt.Errorf("Undefined flag for title")
		}

		dest := filepath.Join(*output, title+".mp4")

		if common.FileExists(dest) {
			err := common.FileBackup(dest)
			if common.Error(err) {
				return err
			}
		}

		return encode(source, "", dest)
	}

	dvdTitle, doc, err := readMetadata(source)
	if common.Error(err) {
		return err
	}

	common.Info("")
	common.Info("Metadata title: %s", dvdTitle)

	if title == "" {
		title = dvdTitle
	}

	title = strings.ReplaceAll(title, "_", " ")

	common.Info("")
	common.Info("Used Title: %s", title)

	rootElem := doc.SelectElement("lsdvd")
	allStart := time.Now()

	index := 0

	for _, trackElem := range rootElem.SelectElements("track") {
		common.Info("")

		indexElem := trackElem.FindElement("ix")
		lengthElem := trackElem.FindElement("length")
		widthElem := trackElem.FindElement("width")

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

		width, err := strconv.Atoi(widthElem.Text())
		if common.Error(err) {
			return err
		}

		if width <= 720 {
			*preset = "Fast 720p30"
		} else {
			*preset = "Fast 1080p30"
		}

		index++

		filename := common.CleanPath(filepath.Join(*output, title, title+" - "+fmt.Sprintf("%02d", index)+"."+ext))

		if common.FileExists(filename) {
			common.Info("target file %s already exists -> skip!", filename)

			continue
		}

		err = os.MkdirAll(filepath.Dir(filename), common.DefaultDirMode)
		if common.Error(err) {
			return err
		}

		err = encode(source, indexElem.Text(), filename)
		if common.Error(err) {
			return err
		}
	}

	common.Info("")
	common.Info("Total time needed: %v\n\n", time.Since(allStart))

	err = eject(source)
	if common.Error(err) {
		return err
	}

	return nil
}

func run() error {
	if len(input) == 1 && input[0] == dvd {
		file := common.CleanPath(input[0])

		err := process(file, "")
		if common.Error(err) {
			return err
		}

	} else {
		for i := 0; i < len(input); i++ {
			file := common.CleanPath(input[i])

			t := ""
			if i < len(title) {
				t = title[i]
			}

			err := process(file, t)
			if common.Error(err) {
				return err
			}
		}
	}

	return nil
}

func main() {
	common.Run([]string{"i"})
}
