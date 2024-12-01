package main

import (
	"errors"
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
		// Don't panic if we're adding a file and the archive doesn't exist.
		// In that case, we're creating a new archive.
		if !*optAdd || !errors.Is(err, os.ErrNotExist) {
			panic(err)
		}
	} else {
		defer zf.Close()
	}

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
		for _, arg := range args[1:] {
			if zf == nil {
				zf, err = zip.Create(args[0], arg, zip.COMPRESS_STORED)
				if err != nil {
					panic(err)
				}
				defer zf.Close()
			} else {
				panicOnError(zf.AddFile(arg, zip.COMPRESS_STORED))
			}
		}
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
