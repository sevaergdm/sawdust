package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sevaergdm/sawdust"
)

func cmdCat(args []string) {
	fs := flag.NewFlagSet("cat", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() != 2 {
		fmt.Fprintf(os.Stderr, "usage: sawdust cat <file> <column>\n")
		os.Exit(2)
	}

	path := fs.Arg(0)
	col := fs.Arg(1)
	file, err := sawdust.OpenFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not read %q: %v\n", path, err)
		os.Exit(1)
	}
	defer func() { _ = file.Close() }()

	vals, err := file.ReadInt64Column(col)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	for _, v := range vals {
		fmt.Println(v)
	}
}
