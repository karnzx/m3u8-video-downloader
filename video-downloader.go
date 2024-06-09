package main

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/grafov/m3u8"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

var BASE_DIRECTORY = "tmp"
var VIDEO_EXT = "mp4"

func clearScreen() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}
func requestURL(url string) (*http.Response, *http.Request) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		panic(err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:47.0) Gecko/20100101 Firefox/47.0")
	req.Header.Set("Accept", "*/*")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("Error making request:", err)
		panic(err)
	}
	return resp, req
}
func getPlayList(url string) (playlist m3u8.Playlist, listType m3u8.ListType, err error) {
	resp, _ := requestURL(url)
	defer resp.Body.Close()
	// fmt.Println(resp.Body)

	fmt.Println("Response status:", resp.Status)
	// fmt.Println("Response Headers:", resp.Header)

	playlist, listType, err = m3u8.DecodeFrom(io.MultiReader(resp.Body), false)
	return playlist, listType, err
}

func downloadVideo(url string) {
	p, listType, err := getPlayList(url)
	if err != nil {
		panic(err)
	}

	switch listType {
	case m3u8.MEDIA:
		mediaPlaylist := p.(*m3u8.MediaPlaylist)
		// fmt.Printf("%+v\n", mediaPlaylist)
		mediaCount := mediaPlaylist.Count()

		for _, segment := range mediaPlaylist.Segments {
			if segment != nil {
				fmt.Printf("Downloading... [%d/%d] \n", segment.SeqId+1, mediaCount)
				// fmt.Println(segment.URI)
				resp, req := requestURL(segment.URI)
				defer resp.Body.Close()

				fileName := fmt.Sprintf("%s/%s", BASE_DIRECTORY, path.Base(req.URL.Path))
				newFileName := fmt.Sprintf("%s.%s", strings.TrimSuffix(fileName, path.Ext(fileName)), VIDEO_EXT)
				f, _ := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY, 0644)
				defer f.Close()
				io.Copy(f, resp.Body)

				err := ffmpeg.Input(fileName).
					Output(newFileName, ffmpeg.KwArgs{"c": "copy"}).
					OverWriteOutput().ErrorToStdOut().Run()
				if err != nil {
					fmt.Println("Error Transcoding to mp4:", err)
					panic(err)
				}

				os.Remove(fileName)
				clearScreen()
			}
		}
	case m3u8.MASTER:
		masterPlaylist := p.(*m3u8.MasterPlaylist)
		// fmt.Printf("%+v\n", masterPlaylist)
		for _, variant := range masterPlaylist.Variants {
			if variant != nil {
				// Display the resolution for each URL
				url := variant.URI
				resolution := variant.Resolution
				fmt.Printf("URL: %s, Resolution: %s\n", url, resolution)

				// Request the URL to show media playlist links
				downloadVideo(url)
				break // select only first video resolution 1080p
			}
		}
	}
}

// return list of files in directory that match file extension
func findFiles(dir string, ext string) []string {
	var a []string
	filepath.WalkDir(dir, func(s string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if filepath.Ext(d.Name()) == ext {
			a = append(a, s)
		}
		return nil
	})
	// sort file name by sequence number after 'p'
	sort.Slice(a, func(i, j int) bool {
		// Extract the number part from the filename
		f := path.Base(a[i])                                // get basepath (ignore directory)
		f = strings.TrimSuffix(f, path.Ext(f))              // ignore ext
		num1, err := strconv.Atoi(strings.Split(f, "p")[1]) // get number after 'p' and convert to integer
		if err != nil {
			panic(err)
		}

		f = path.Base(a[j])
		f = strings.TrimSuffix(f, path.Ext(f))
		num2, err := strconv.Atoi(strings.Split(f, "p")[1])
		if err != nil {
			panic(err)
		}
		return num1 < num2
	})

	return a
}

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Require 2 arguments: %s <m3u8_url> <output_file_name> \n", os.Args[0])
		os.Exit(1)
	}
	url := os.Args[1]
	outputFile := os.Args[2]
	outputFile = fmt.Sprintf("%s.%s", outputFile, VIDEO_EXT)

	os.Mkdir(BASE_DIRECTORY, 0777)

	downloadVideo(url)

	// Create video list from segments
	videoListName := "list.txt"
	f, _ := os.OpenFile(videoListName, os.O_CREATE|os.O_WRONLY, 0644)
	for _, s := range findFiles(BASE_DIRECTORY, ".mp4") {
		// println(s)
		io.WriteString(f, fmt.Sprintf("file %s \n", s))
	}
	defer f.Close()

	// Join video segments
	err := ffmpeg.Input(videoListName, ffmpeg.KwArgs{"f": "concat"}).
		Output(outputFile, ffmpeg.KwArgs{"c": "copy"}).
		OverWriteOutput().ErrorToStdOut().Run()
	if err != nil {
		fmt.Println("Error Transcoding to mp4:", err)
		panic(err)
	}

	os.RemoveAll(BASE_DIRECTORY)
}
