package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"os"

	"github.com/targodan/goad"
)

func panicOnErr(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	// Handle missuse.
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintln(os.Stderr, "Usage: go run decode.go <audiofile> [outfile]")
		os.Exit(-1)
	}

	inFilename := os.Args[1]
	var outFilename string
	if len(os.Args) == 3 {
		outFilename = os.Args[2]
	} else {
		outFilename = os.Args[1] + ".raw"
	}

	// Open the outFile
	outFile, err := os.OpenFile(outFilename, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	panicOnErr(err)
	// Remember to close it afterwards.
	defer outFile.Close()

	decoder := goad.NewDecoder()
	defer decoder.Close()

	ch, err := decoder.EnableFirstAudioStream(2048)
	panicOnErr(err)

	decoder.Start()

	bufFile := bufio.NewWriter(outFile)
	defer bufFile.Flush()
	for _, samples := range ch {
		for sample := range samples {
			binary.Write(bufFile, binary.LittleEndian, sample)
		}
	}
}