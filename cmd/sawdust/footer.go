package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sevaergdm/sawdust"
)

func cmdFooter(args []string) {
	fs := flag.NewFlagSet("footer", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: sawdust footer <file>\n")
		os.Exit(2)
	}

	path := fs.Arg(0)
	file, err := sawdust.OpenFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not read %q: %v\n", path, err)
		os.Exit(1)
	}
	defer func() { _ = file.Close() }()

	fmt.Printf("file size: %d bytes\n", file.Size)
	fmt.Printf("footer length: %d bytes\n", file.Footer.Length)
	fmt.Printf("metadata start position: %d\n", file.Footer.Start)
	fmt.Printf("num rows: %d\n", file.Metadata.NumRows)
	fmt.Printf("version: %d\n", file.Metadata.Version)
	fmt.Printf("created by: %q\n", file.Metadata.CreatedByOrEmpty())
}
