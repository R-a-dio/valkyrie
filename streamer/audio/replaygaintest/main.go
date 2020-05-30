package main

import (
	"io"
	"log"
	"os"

	"github.com/R-a-dio/valkyrie/streamer/audio"
)

func main() {
	filename := os.Args[1]

	buf, err := audio.DecodeFileGain(filename)
	if err != nil {
		log.Println("decode:", err)
		return
	}

	output, err := os.Create(filename + ".replaygain.pcm")
	if err != nil {
		log.Println("create:", err)
		return
	}
	defer output.Close()

	_, err = io.Copy(output, buf.Reader())
	if err != nil {
		log.Println("copy:", err)
		return
	}

	log.Println("written:", filename+".replaygain.pcm")
}
