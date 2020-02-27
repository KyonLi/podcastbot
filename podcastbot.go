package main

import (
	"encoding/json"
	"github.com/rylio/ytdl"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
)

func createTempDir() {
	path := "tmp"
	// check
	if _, err := os.Stat(path); err != nil {
		err := os.Mkdir(path, os.ModeDir)
		if err != nil {
			log.Fatal("error creating temp directory")
		}
	}
	// check again
	if _, err := os.Stat(path); err != nil {
		log.Fatal("error creating temp directory")
	}
}

func metadata(full string) (title string, album string) {
	s := strings.Split(full, "/")
	if len(s) == 3 {
		//normal
		return s[0], s[1]
	} else {
		return full, ""
	}
}

func download(u string) (path string, desc string) {

	//get video info
	vid, err := ytdl.GetVideoInfo(u)
	if err != nil {
		log.Printf("failed to get video info (%s)", u)
		return
	}

	//find audio-only source
	var formats []*ytdl.Format
	for _, f := range vid.Formats {
		if f.Itag.AudioEncoding == "aac" && f.Itag.VideoEncoding == "" && f.Itag.Resolution == "" {
			formats = append(formats, f)
		}
	}
	if len(formats) > 0 {
		if len(formats) > 1 {
			//descending sort
			sort.Slice(formats, func(i, j int) bool {
				return formats[i].Itag.AudioBitrate > formats[j].Itag.AudioBitrate
			})
		}

		//download
		path := "tmp/" + vid.ID + ".m4a"
		file, _ := os.Create(path)
		err = vid.Download(formats[0], file)
		file.Close()
		if err != nil {
			log.Printf("%s - failed to download audio", vid.ID)
			return "", ""
		}

		//write metadata
		newPath := "tmp/" + vid.ID + "-new.m4a"
		title, album := metadata(vid.Title)
		cmd := exec.Command("ffmpeg", "-y", "-i", path, "-metadata", "title=" + title, "-metadata", "artist=" + vid.Uploader, "-metadata", "album=" + album, "-codec", "copy", newPath)
		//log.Printf("run: %s %s\n", cmd.Path, cmd.Args)
		_, err = cmd.CombinedOutput()
		//log.Print(string(out))
		if err != nil {
			log.Println(err)
			os.Remove(newPath)
		} else {
			os.Rename(newPath, path)
		}
		return path, title + "\n\n" + vid.Description
	} else {
		log.Printf("%s - audio not found", vid.ID)
		return "", ""
	}
}

func runWebApi()  {
	http.HandleFunc("/podcastbot", func(w http.ResponseWriter, r *http.Request) {
		jsonData := make(map[string]string)
		err := json.NewDecoder(r.Body).Decode(&jsonData)
		if err != nil {
			log.Print("parse request error", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("bad request"))
			return
		}
		u, ok := jsonData["url"]
		if ok && u != "" {
			go func(u string) {
				p, d := download(u)
				log.Print(p, d)
			}(u)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		} else {
			log.Print("invalid request body", jsonData)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("bad request"))
		}
	})
	err := http.ListenAndServe(":9090", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func main() {
	createTempDir()
	runWebApi()
}
