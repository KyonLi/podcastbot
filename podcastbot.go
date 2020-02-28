package main

import (
	"encoding/json"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/rylio/ytdl"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
)

const BotToken = ""

var Bot *tgbotapi.BotAPI

var ChannelMap = map[string]int64{}

type AudioFile struct {
	Path      string
	Title     string
	Artist    string
	Duration  int
	Desc      string
	ChannelID int64
}

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

func cutCaption(str string) string {
	rs := []rune(str)
	if len(rs) > 1023 {
		return string(rs[:1023]) + "…"
	} else {
		return str
	}
}

func download(u string, channelID int64) *AudioFile {

	//get video info
	vid, err := ytdl.GetVideoInfo(u)
	if err != nil {
		log.Printf("failed to get video info (%s)", u)
		return nil
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
			os.Remove(path)
			log.Printf("%s - failed to download audio", vid.ID)
			return nil
		}

		//write metadata
		newPath := "tmp/" + vid.ID + "-new.m4a"
		title, album := metadata(vid.Title)
		cmd := exec.Command("ffmpeg", "-y", "-i", path, "-metadata", "title="+title, "-metadata", "artist="+vid.Uploader, "-metadata", "album="+album, "-codec", "copy", newPath)
		//log.Printf("run: %s %s\n", cmd.Path, cmd.Args)
		_, err = cmd.CombinedOutput()
		//log.Print(string(out))
		if err != nil {
			log.Println(err)
			os.Remove(newPath)
		} else {
			os.Rename(newPath, path)
		}
		return &AudioFile{
			Path:      path,
			Title:     title,
			Artist:    vid.Uploader,
			Duration:  int(vid.Duration.Seconds()),
			Desc:      title + "\n\n" + vid.Description,
			ChannelID: channelID,
		}
	} else {
		log.Printf("%s - audio not found", vid.ID)
		return nil
	}
}

func runWebApi() {
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
			cid, ok := jsonData["cid"]
			if ok && cid != "" {
				go func(u string, cid string) {
					a := download(u, ChannelMap[cid])
					if a != nil {
						sendAudio(a)
					}
				}(u, cid)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ok"))
				return
			}
		}
		log.Print("invalid request body", jsonData)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	})
	err := http.ListenAndServe(":9090", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func initBot() {
	bot, err := tgbotapi.NewBotAPI(BotToken)
	if err != nil {
		log.Fatal("failed to init bot", err)
	}
	bot.Debug = false
	Bot = bot
}

func sendAudio(a *AudioFile) {
	msg := tgbotapi.NewAudioUpload(a.ChannelID, a.Path)
	msg.Title = a.Title
	msg.Performer = a.Artist
	msg.Duration = a.Duration
	msg.Caption = cutCaption(a.Desc)
	Bot.Send(msg)
	os.Remove(a.Path)
}

func main() {
	createTempDir()
	initBot()
	runWebApi()
}
