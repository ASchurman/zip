package main

import (
	"flag"
	"fmt"
)

func main() {
	table := flag.Bool("t", false, "display table of contents")
	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		fmt.Println("Usage: go run main.go [-t] <zip file>")
		return
	}

	zf, err := openZipFile(args[0])
	if err != nil {
		panic(err)
	}
	if zf == nil {
		panic("openZipFile returned nil without having an error")
	}
	defer zf.close()

	if *table {
		zf.display()
	}
}
