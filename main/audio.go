package main

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"

	// "os/signal"
	"strings"
	"text/template"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/gordonklaus/portaudio"
	"github.com/sunicy/go-lame"
)

// Install portaudio and mp3lame
//
// Linux:
// sudo apt-get install portaudio19-dev libmp3lame-dev
//
// MacOS:
// brew install lame
// brew install pkg-config
// export CGO_CFLAGS="-I/opt/homebrew/opt/lame/include"
// export CGO_LDFLAGS="-L/opt/homebrew/opt/lame/lib"

// type status struct {
// 	AudioRecordingFile                         string
// 	AudioRecordingLoundness                    float32
// }
// var globalStatus status
// var STRATUX_HOME = "/opt/stratux/"
// func main() {
// 	sig := make(chan os.Signal, 1)
// 	signal.Notify(sig, os.Interrupt, os.Kill)
// 	go initPortAudio()

// 	for {
// 		select {
// 		case <-sig:
// 			log.Printf("Sig")
// 			return
// 		default:
// 		}
// 	}
//  }

func initAudio() {
	timer := time.NewTicker(10 * time.Second)
	for {
		<-timer.C

		// If it's not currently recording, try start when we have a valid system time
		if isGPSClockValid() && globalSettings.AudioRecordingEnabled && len(globalStatus.AudioRecordingFile) == 0 {
			go initPortAudio()
		}
	}
}

func getCurrentFilename() (string) {
	startTime := time.Now()
	mp3FileName := startTime.Format("2006-01-02-150405") + ".mp3"
	return mp3FileName
}

func initMp3Output(inSampleRate int, mp3File *os.File) (*lame.Writer) {

	pcmWriter, errWriter := lame.NewWriter(mp3File)
	if errWriter != nil {
		log.Printf("Error initializing lame writer: %s\n", errWriter.Error())
		return nil
	}

	// encoding settings
	pcmWriter.EncodeOptions.InNumChannels = 1
	pcmWriter.EncodeOptions.InSampleRate = inSampleRate
	pcmWriter.EncodeOptions.OutSampleRate = 16000
	pcmWriter.EncodeOptions.OutQuality = 6
	pcmWriter.ForceUpdateParams()
	
	return pcmWriter
}

func initPortAudio() {
	log.Println("Initializing portaudio")
	errInit := portaudio.Initialize()
	if errInit != nil {
		log.Printf("Error initializing portaudio: %s\n", errInit.Error())
		return
	}
	defer log.Println("Deinitializing portaudio")
	defer portaudio.Terminate()

	inSampleRate := 44100

	buffer := make([]int16, inSampleRate)

	log.Println("Open portaudio stream")

	stream, streamErr := portaudio.OpenDefaultStream(
		1, 
		0, 
		float64(inSampleRate),
		len(buffer),
		buffer,
	)
	if streamErr != nil {
		log.Printf("Error initializing portaudio stream: %s\n", streamErr.Error())
		return
	}

	startErr := stream.Start()
	if startErr != nil {
		log.Printf("Error starting portaudio stream: %s\n", startErr.Error())
		return
	}
	defer stream.Close()

	log.Println("Opened portaudio stream")

	// wait until we receive any sound
	globalStatus.AudioRecordingFile = "waiting"
	globalStatus.AudioRecordingLoundness = loudness(&buffer)

	log.Printf("Start waiting for portaudio sound, starting point %.1f db\n", globalStatus.AudioRecordingLoundness)

	for globalSettings.AudioRecordingEnabled && globalStatus.AudioRecordingLoundness < -50 {
		err := stream.Read()
		if err != nil {
			log.Printf("Error reading stream: %s\n", err.Error())
			continue;
		}

		globalStatus.AudioRecordingLoundness = loudness(&buffer)	
		log.Printf("Waiting portaudio sound, heard %.1f db\n", globalStatus.AudioRecordingLoundness)
	}

	log.Printf("Done waiting for portaudio sound, now %.1f db\n", globalStatus.AudioRecordingLoundness)

	if globalStatus.AudioRecordingLoundness > -50 {
		log.Println("Audio recording starting")
		mp3FileName := getCurrentFilename()
		mp3File, _ := os.Create(STRATUX_HOME + "/audio/" + mp3FileName)
		defer mp3File.Close()
		globalStatus.AudioRecordingFile = mp3FileName
		
		pcmWriter := initMp3Output(inSampleRate, mp3File)
		if pcmWriter == nil {
			return
		}
		defer pcmWriter.Close()

		log.Printf("Audio recording started to %s\n", mp3FileName)
	
		// keep looping until disabled
		for globalSettings.AudioRecordingEnabled {
			err := stream.Read()
			if err != nil {
				log.Printf("Error reading stream: %s\n", err.Error())
				continue;
			}
	
			globalStatus.AudioRecordingLoundness = loudness(&buffer)	
			binary.Write(pcmWriter, binary.LittleEndian, buffer)
		}

	}

	// cleanup
	globalStatus.AudioRecordingFile = ""
	globalStatus.AudioRecordingLoundness = 0
	log.Println("Audio recording stopped")
}

func loudness(buffer *[]int16) float32 {
	amplitude := int16(0)
	for i, a := range *buffer {
		if i==0 || a > amplitude {
			amplitude = a
		}
	}

	return float32(20 * math.Log10(float64(amplitude) / 32767.0))
}

func handleAudioStream(w http.ResponseWriter, r *http.Request) {
	file := r.URL.Query().Get("file")

	path := STRATUX_HOME + "/audio/" + file
	log.Printf("Starting client #%v", path)	
	http.ServeFile(w, r, path)
}

func viewAudioRecordings(w http.ResponseWriter, r *http.Request) {
	urlpath := strings.TrimPrefix(r.URL.Path, "/audio/")
	path := STRATUX_HOME + "/audio/" + urlpath
	finfo, err := os.Stat(path)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("Failed to open %s: %s", path, err.Error())))
		return
	}

	if !finfo.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	
	names, err := ioutil.ReadDir(path)
	if err != nil {
		return
	}	

	fi := make([]fileInfo, 0)
	for _, val := range names {
		if val.Name()[0] == '.' {
			continue
		} // Remove hidden files from listing
		
		if val.IsDir() {
			mtime := val.ModTime().Format("2006-Jan-02 15:04:05")
			sz := ""
			fi = append(fi, fileInfo{Name: val.Name() + "/", Path: urlpath + "/" + val.Name(), Mtime: mtime, Size: sz})
		} else {
			mtime := val.ModTime().Format("2006-Jan-02 15:04:05")
			sz := humanize.Comma(val.Size())
			fi = append(fi, fileInfo{Name: val.Name(), Path: urlpath + "/" + val.Name(), Mtime: mtime, Size: sz})
		}
	}

	tpl, err := template.New("tpl").Parse(strings.Replace(dirlisting_tpl, "/logs/", "/audio/", -1))
	if err != nil {
		return
	}
	data := dirlisting{Name: r.URL.Path, ServerUA: "Stratux " + stratuxVersion + "/" + stratuxBuild,
		Children_files: fi}

	err = tpl.Execute(w, data)
	if err != nil {
		log.Printf("viewAudioRecordings() error: %s\n", err.Error())
	}

}