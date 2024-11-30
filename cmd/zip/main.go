package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ASchurman/zip"
)

func main() {
	optTable := flag.Bool("t", false, "display table of contents")
	optExtract := flag.Bool("x", false, "extract a file (or, if no file is specified, extract all files)")
	optAdd := flag.Bool("r", false, "add a file to the zip file")
	optDelete := flag.Bool("d", false, "delete a file from the zip file")
	flag.Parse()
	args := flag.Args()

	if len(args) == 0 || flag.NFlag() != 1 {
		fmt.Println("Usage: zip {-d|-r|-t|-x} ARCHIVE [FILE ...]")
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
	if *optTable {
		zf.Display(os.Stdout)
	} else if *optExtract {
		if len(args) > 1 {
			for _, arg := range args[1:] {
				panicOnError(zf.ExtractFile(arg))
			}
		} else {
			panicOnError(zf.ExtractAll())
		}
	} else if *optAdd {
		panic("Not implemented")
	} else if *optDelete {
		for _, arg := range args[1:] {
			panicOnError(zf.RemoveFile(arg))
		}
	}
}

func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}
