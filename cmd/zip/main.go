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

	file, err := os.Open(args[0])
	if err != nil {
		panic(err)
	}
	defer file.Close()

	zd, err := zip.NewZipDir(args[0], file)
	if err != nil {
		panic(err)
	}
	if zd == nil {
		panic("NewZipDir returned nil without having an error")
	}

	if *table {
		zd.Display(os.Stdout)
	}
}
