package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
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
// export CGO_CFLAGS="-I/opt/homebrew/opt/lame/include"
// export CGO_LDFLAGS="-L/opt/homebrew/opt/lame/lib"

var audioStreamer *streamer

func initAudio() {
	timer := time.NewTicker(10 * time.Second)
	for {
		<-timer.C

		// If it's not currently recording, try start
		if globalSettings.AudioRecordingEnabled && len(globalStatus.AudioRecordingFile) == 0 {
			go initPortAudio()
		}
	}
}

func initPortAudio() {
	portaudio.Initialize()
	defer portaudio.Terminate()

	startTime := time.Now()
	mp3FileName := startTime.Format("2006-01-02-150405") + ".mp3"
	mp3File, _ := os.Create(STRATUX_HOME + "/audio/" + mp3FileName)
	defer mp3File.Close()
	log.Println("Audio output to", mp3FileName)

	// mp3 output is written into disk and streamed
	mp3PipeReader, mp3PipeWriter := io.Pipe()
	writers := io.MultiWriter(mp3PipeWriter, mp3File)
	pcmWriter, err := lame.NewWriter(writers)
	if err != nil {
		log.Printf("Error initializing lame writer: %s\n", err.Error())
		return
	}

	// encoding settings
	pcmWriter.EncodeOptions.InNumChannels = 1
	pcmWriter.EncodeOptions.InSampleRate = 44100
	pcmWriter.EncodeOptions.OutSampleRate = 16000
	pcmWriter.EncodeOptions.OutQuality = 6
	pcmWriter.ForceUpdateParams()
	defer pcmWriter.Close()

	stream, err := portaudio.OpenDefaultStream(
		1, 
		0, 
		float64(pcmWriter.EncodeOptions.InSampleRate), 
		pcmWriter.EncodeOptions.InSampleRate / 2, // half a second buffer 
		func(in []int16) {
			globalStatus.AudioRecordingLoundness = loudness(&in)
			//fmt.Printf("%.1f db\n", globalStatus.AudioRecordingLoundness)
			binary.Write(pcmWriter, binary.LittleEndian, in)
		})
	if err != nil {
		log.Printf("Error initializing portaudio stream: %s\n", err.Error())
		return
	}

	err = stream.Start()
	if err != nil {
		log.Printf("Error starting portaudio stream: %s\n", err.Error())
		return
	}
	defer stream.Close()

	globalStatus.AudioRecordingFile = mp3FileName
	log.Println("Audio recording started")

	audioStreamer = new(streamer)
	audioStreamer.Input = mp3PipeReader
	// how much to read from mp3 stream at once
	audioStreamer.ReadBuff = 4000 // read buffer size
	audioStreamer.QueueSize = 10 // queue size
	audioStreamer.WriteBuff = 32768 // write buffer size
	err = audioStreamer.init()
	if err != nil {
		log.Fatalln(err)
		return
	}

	// keep looping until disabled
	audioStreamer.readLoop()

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

type streamer struct {
	sync.RWMutex
	clients   map[uint64]chan []byte
	id        uint64
	ReadBuff  int
	QueueSize int
	WriteBuff int
	Input     io.Reader
	skipped   *int
	Stop      chan bool
}

func (s *streamer) init() (err error) {
	s.Lock()
	defer s.Unlock()
	s.skipped = new(int)
	s.clients = make(map[uint64]chan []byte)
	s.Stop = make(chan bool)
	
	if err != nil {
		return
	}
	return
}

func (s *streamer) addClient() (uint64, chan []byte) {
	s.Lock()
	defer s.Unlock()
	s.id++
	s.clients[s.id] = make(chan []byte, s.QueueSize)
	return s.id, s.clients[s.id]
}

func (s *streamer) delClient(id uint64) {
	log.Printf("Deleting client #%v", id)

	s.Lock()
	defer s.Unlock()
	close(s.clients[id])
	delete(s.clients, id)
}

func (s *streamer) send(b []byte) {
	s.RLock()
	defer s.RUnlock()
	for _, v := range s.clients {
		select {
		case v <- b:
		default:
		}
	}
}

func (s *streamer) readLoop() {
	defer close(s.Stop)
	for {
		if !globalSettings.AudioRecordingEnabled {
			return
		}

		buffer := make([]byte, s.ReadBuff)
		_, err := io.ReadFull(s.Input, buffer)
		if err != nil {
			log.Println(err)
			return
		}
		s.send(buffer)
	}
}

func handleAudioStream(w http.ResponseWriter, r *http.Request) {
	id, recieve := audioStreamer.addClient()
	defer audioStreamer.delClient(id)

	log.Printf("Starting client #%v", id)

	// Set some headers
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Server", "dumb-mp3-streamer")
	//Send MP3 stream header
	head := []byte{0x49, 0x44, 0x33, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	//Send data in chunks
	buffw := bufio.NewWriterSize(w, audioStreamer.WriteBuff)
	if _, err := buffw.Write(head); err != nil {
		return
	}

	for {
		chunk := <-recieve
		if _, err := buffw.Write(chunk); err != nil {
			return
		}
	}
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