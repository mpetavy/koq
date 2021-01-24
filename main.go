package main

import (
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
)

func init() {
	common.Init(false, "1.0.0", "", "2021", "Rescues my KOQ discs", "mpetavy", fmt.Sprintf("https://github.com/mpetavy/%s", common.Title()), common.APACHE, nil, nil, run, 0)

	minLength = flag.Duration("min", time.Minute*10, "minimum duration to consider as valid track")
	preset = flag.String("p", "Fast 720p30", "device to read the DVD content")
	handbrake = flag.String("h", "HandBrakeCLI", "path to Handbrake CLI executable")
	device = flag.String("d", "/dev/dvd", "device to read the DVD content")
	format = flag.String("f", "av_mp4", "Handbrake video format")
	videoEncoder = flag.String("v", "nvenc_h264", "Handbrake video encoder")
	audioEncoder = flag.String("a", "copy:ac3", "Handbrake audio encoder")
	language = flag.String("l", "de", "Handbrake language")
	output = flag.String("o", ".", "Output directory")
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

	cmd := exec.Command("lsdvd", "-Ox", "-a", "-v")
	ba, err := cmd.Output()
	if common.Error(err) {
		return err
	}

	doc := etree.NewDocument()
	err = doc.ReadFromBytes(ba)
	if common.Error(err) {
		return err
	}

	rootElem := doc.SelectElement("lsdvd")

	titleElem := rootElem.FindElement("//lsdvd/title")
	if titleElem == nil {
		return fmt.Errorf("cannot find title element")
	}

	index := 0
	for _, trackElem := range rootElem.SelectElements("track") {
		indexElem := trackElem.FindElement("ix")
		lengthElem := trackElem.FindElement("length")

		secs, err := strconv.ParseFloat(lengthElem.Text(), 64)
		if common.Error(err) {
			return err
		}

		secsDuration := time.Second * time.Duration(secs)

		fmt.Printf("Track %s: %v\n", indexElem.Text(), secsDuration)

		if secsDuration < *minLength {
			fmt.Printf("track too short  -> skip!\n\n")

			continue
		}

		ext := *format
		p := strings.LastIndex(*format, "_")
		if p != -1 {
			ext = ext[p+1:]
		}

		title := titleElem.Text()
		ss := strings.Split(title, "_")

		var sb strings.Builder

		for _, s := range ss {
			if sb.Len() > 0 {
				sb.WriteString(" ")
			}

			sb.WriteString(common.Capitalize(strings.ToLower(s)))
		}

		title = sb.String()

		index++
		filename := common.CleanPath(filepath.Join(*output, title, title+" - "+fmt.Sprintf("%02d", index)+"."+ext))

		b, _ = common.FileExists(filename)

		if b {
			fmt.Printf("target file %s already exists -> skip!\n\n", filename)

			continue
		}

		err = os.MkdirAll(filepath.Dir(filename), common.DefaultDirMode)
		if common.Error(err) {
			return err
		}

		fmt.Printf("Start: %v\n", time.Now().Format(common.DateTimeMask))

		cmd = exec.Command(*handbrake,
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

		fmt.Println(common.CmdToString(cmd))

		start := time.Now()

		err = cmd.Run()
		if common.Error(err) {
			return err
		}

		fmt.Printf("End: %v\n", time.Now().Format(common.DateTimeMask))

		fmt.Printf("Time needed: %v\n\n", time.Since(start))
	}

	return nil
}

func main() {
	defer common.Done()

	common.Run(nil)
}
