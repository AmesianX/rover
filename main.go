package main

import (
	"archive/zip"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/cj123/ranger"
	"github.com/dustin/go-humanize"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	sourceURL  string
	remoteFile string
	localFile  string
	timeout    int
	verbose    bool
	showFiles  bool
)

func init() {
	flag.StringVar(&sourceURL, "u", "", "the url you wish to download from")
	flag.StringVar(&remoteFile, "r", "", "the remote filename to download")
	flag.StringVar(&localFile, "o", "", "the output filename")
	flag.IntVar(&timeout, "t", 5, "timeout, in seconds")
	flag.BoolVar(&verbose, "v", false, "verbose")
	flag.BoolVar(&showFiles, "l", false, "list files in zip")

	flag.Parse()

	if sourceURL == "" {
		fmt.Println("You must specify a URL")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if !showFiles {
		if remoteFile == "" {
			fmt.Println("You must specify a remote filename")
			flag.PrintDefaults()
			os.Exit(1)
		}

		if localFile == "" {
			localFile = remoteFile[:len(filepath.Base(remoteFile))]
		}
	}
}

// returns a progress bar fitting the terminal width given a progress percentage
func progressBar(progress int) (progressBar string) {

	var width int

	if runtime.GOOS == "windows" {
		// we'll just assume it's standard terminal width
		width = 80
	} else {
		width, _, _ = terminal.GetSize(0)
	}

	// take off 40 for extra info (e.g. percentage)
	width = width - 40

	// get the current progress
	currentProgress := (progress * width) / 100

	progressBar = "["

	// fill up progress
	for i := 0; i < currentProgress; i++ {
		progressBar = progressBar + "="
	}

	progressBar = progressBar + ">"

	// fill the rest with spaces
	for i := width; i > currentProgress; i-- {
		progressBar = progressBar + " "
	}

	// end the progressbar
	progressBar = progressBar + "] " + fmt.Sprintf("%3d", progress) + "%"

	return progressBar
}

func downloadFile(file *zip.File, writer *os.File) error {
	errCh := make(chan error)

	go func() {
		rc, err := file.Open()

		if err != nil {
			errCh <- err
			return
		}

		defer rc.Close()

		buf := make([]byte, 128*1024)

		downloaded := uint64(0)
		filesize := file.UncompressedSize64
		humanizedFilesize := humanize.Bytes(filesize)

		for {
			if n, _ := io.ReadFull(rc, buf); n > 0 {
				writer.Write(buf[:n])
				downloaded += uint64(n)

				if verbose {
					fmt.Printf("\r%s %10s/%-10s", progressBar(int(downloaded*100/filesize)), humanize.Bytes(downloaded), humanizedFilesize)
				}
			} else {
				break
			}
		}

		if verbose {
			fmt.Println()
		}

		errCh <- nil
	}()

	return <-errCh
}

func findFile(reader *zip.Reader, filename string) (*zip.File, error) {
	if reader.File == nil {
		return nil, errors.New("file read error")
	}

	for _, f := range reader.File {
		if f.Name == filename {
			return f, nil
		}
	}

	return nil, errors.New("Unable to find file")
}

func listFiles(reader *zip.Reader) error {
	if reader.File == nil {
		return errors.New("file read error")
	}

	for _, f := range reader.File {
		fmt.Println(f.Name)
	}

	return nil
}

func main() {
	downloadURL, err := url.Parse(sourceURL)

	reader, err := ranger.NewReader(
		&ranger.HTTPRanger{
			URL: downloadURL,
			Client: &http.Client{
				Timeout: time.Duration(timeout) * time.Second,
			},
		},
	)

	if err != nil {
		fmt.Printf("Unable to create reader for url: %s\n", downloadURL)
		os.Exit(1)
	}

	zipreader, err := zip.NewReader(reader, reader.Length())

	if err != nil {
		fmt.Printf("Unable to create zip reader for url: %s\n", downloadURL)
		os.Exit(1)
	}

	if showFiles {
		listFiles(zipreader)
		return
	}

	var localFileHandle *os.File

	if localFile != "-" {
		localFileHandle, err = os.Create(localFile)
	} else {
		localFileHandle = os.Stdout
	}

	defer localFileHandle.Close()

	if err != nil {
		fmt.Printf("Unable to create local file: %s", localFile)
		os.Exit(1)
	}

	foundFile, err := findFile(zipreader, remoteFile)

	if err != nil {
		fmt.Printf("Unable find file: %s in zip.", remoteFile)
		os.Exit(1)
	}

	err = downloadFile(foundFile, localFileHandle)

	if err != nil {
		fmt.Printf("Unable read file %s from zip.", remoteFile)
		os.Exit(1)
	}
}