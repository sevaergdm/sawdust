package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/sevaergdm/sawdust"
)

func cmdSurvey(args []string) {
	fl := flag.NewFlagSet("survey", flag.ExitOnError)
	_ = fl.Parse(args)

	if fl.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: sawdust survey <dir>\n")
		os.Exit(2)
	}

	t := newTotals()
	path := fl.Arg(0)
	err := filepath.WalkDir(path, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if de.IsDir() {
			return nil
		}

		if !strings.HasSuffix(p, ".parquet") {
			t.skipped++
			return nil
		}

		file, err := sawdust.ReadFile(p)
		if err != nil {
			return err
		}

		t.add(file)
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "encountered an error walking %s: %v\n", path, err)
		os.Exit(1)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintf(w, "files: %d\tskipped: %d\trows: %d\trow groups: %d\trows/group: min %d mean %d max %d\ttotal size: %d bytes\n", t.files, t.skipped, t.numRows, t.numRowGroups, t.minRowsPerGroup, t.numRows/int64(t.numRowGroups), t.maxRowsPerGroup, t.fileBytes); err != nil {
		fmt.Fprintf(os.Stderr, "encountered an error printing file summary: %v", err)
		os.Exit(1)
	}

	if _, err := fmt.Fprintln(w, ""); err != nil {
		fmt.Fprintf(os.Stderr, "encountered an error printing blank line: %v", err)
		os.Exit(1)
	}

	if _, err := fmt.Fprintln(w, "by compressed bytes:"); err != nil {
		fmt.Fprintf(os.Stderr, "encountered an error printing compressed bytes list message: %v", err)
		os.Exit(1)
	}

	if _, err := fmt.Fprintln(w, "\tColumn\tCompressed\tUncompressed\tCompressed Ratio"); err != nil {
		fmt.Fprintf(os.Stderr, "encountered an error printing compressed bytes list header: %v", err)
		os.Exit(1)
	}

	sortedColumns := t.ranking()
	for _, col := range sortedColumns {
		r := compressedRatio(col.compressedBytes, col.uncompressedBytes)
		if _, err := fmt.Fprintf(w, "\t%s\t%d\t%d\t%s\n", col.name, col.compressedBytes, col.uncompressedBytes, r); err != nil {
			fmt.Fprintf(os.Stderr, "encountered an error printing sorted column row: %v", err)
			os.Exit(1)
		}
	}

	if err := w.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "encountered an error printing column report: %v", err)
		os.Exit(1)
	}

}
