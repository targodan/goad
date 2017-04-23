// MIT License
//
// Copyright (c) 2016 Luca Corbatto
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

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

	decoder, err := goad.NewDecoder(inFilename)
	if err != nil {
		panic(err)
	}
	defer decoder.Close()

	ch, _, err := decoder.EnableFirstAudioStream(2048, 44100)
	panicOnErr(err)

	errCh := decoder.Start()

	go func(errCh <-chan error) {
		for err := range errCh {
			panicOnErr(err)
		}
	}(errCh)

	bufFile := bufio.NewWriter(outFile)
	defer bufFile.Flush()
	for samples := range ch {
		for _, sample := range samples {
			binary.Write(bufFile, binary.LittleEndian, sample)
		}
	}
}
