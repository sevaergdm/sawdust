package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sevaergdm/sawdust"
)

func main() {

	filePath := flag.String("path", "", "path for parquet file to be read")
	flag.Parse()

	if *filePath == "" {
		fmt.Fprintf(os.Stderr, "error: must supply a parquet file to read\n")
		flag.Usage()
		os.Exit(2)
	}

	f, err := os.Open(*filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not open %s: %v\n", *filePath, err)
		os.Exit(1)
	}
	defer func() { _ = f.Close() }()

	fStat, err := f.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not stat %s: %v\n", *filePath, err)
		os.Exit(1)
	}

	size := fStat.Size()

	footer, err := sawdust.ReadFooter(f, size)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: encountered an error reading footer in %s: %v\n", *filePath, err)
		os.Exit(1)
	}

	footerBytes := make([]byte, footer.Length)
	if _, err := f.ReadAt(footerBytes, footer.Start); err != nil {
		fmt.Fprintf(os.Stderr, "error: encountered an error reading from offset in %s: %v\n", *filePath, err)
		os.Exit(1)
	}

	fileMetadata, err := sawdust.ReadFileMetadata(footerBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: encountered an error fetching file metadata in %s: %v\n", *filePath, err)
		os.Exit(1)
	}

	fmt.Printf("file size: %d bytes\n", size)
	fmt.Printf("footer length: %d bytes\n", footer.Length)
	fmt.Printf("metadata start position: %d\n", footer.Start)
	fmt.Printf("num rows: %d\n", fileMetadata.NumRows)
	fmt.Printf("version;: %d\n", fileMetadata.Version)
	fmt.Printf("created by: %q\n", fileMetadata.CreatedByOrEmpty())
}
