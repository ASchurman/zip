package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ASchurman/zip"
)

func main() {
	table := flag.Bool("t", false, "display table of contents")
	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		fmt.Println("Usage: go run main.go [-t] <zip file>")
		return
	}

	// Open zip file
	zf, err := zip.Open(args[0])
	if err != nil {
		panic(err)
	}
	if zf == nil {
		panic("NewZipDir returned nil without having an error")
	}
	defer zf.Close()

	// Do the desired operation
	if *table {
		zf.Display(os.Stdout)
	}
}
