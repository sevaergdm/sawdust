package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/sevaergdm/sawdust"
)

func cmdStat(args []string) {
	fs := flag.NewFlagSet("stat", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: sawdust stat <file>\n")
		os.Exit(2)
	}

	t := newTotals()
	path := fs.Arg(0)
	file, err := sawdust.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not read %q: %v\n", path, err)
		os.Exit(1)
	}

	t.add(file)
	rowsPerGroup := buildRowsPerGroup(file.Metadata.RowGroups)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintf(w, "rows: %d  row groups: %d %s file: %d bytes\n", t.numRows, t.numRowGroups, rowsPerGroup, t.fileBytes); err != nil {
		fmt.Fprintf(os.Stderr, "encountered an error printing file summary: %v", err)
		os.Exit(1)
	}

	fmt.Println()

	if _, err := fmt.Fprintln(w, "group\tcolumn\ttype\tcodec\tencodings\tvalues\tcomp\tuncomp\tratio\tnulls"); err != nil {
		fmt.Fprintf(os.Stderr, "encountered an error printing report header: %v", err)
		os.Exit(1)
	}

	for i, group := range file.Metadata.RowGroups {
		for _, c := range group.Columns {
			metadata := c.Metadata

			ratio := compressedRatio(metadata.TotalCompressedSize, metadata.TotalUncompressedSize)
			if _, err := fmt.Fprintf(w, "%d\t%s\t%v\t%v\t%s\t%d\t%d\t%d\t%s\t%s\n",
				i,
				strings.Join(metadata.PathInSchema, "."),
				metadata.Type,
				metadata.Codec,
				encodingNames(metadata.Encodings),
				metadata.NumValues,
				metadata.TotalCompressedSize,
				metadata.TotalUncompressedSize,
				ratio,
				orDash(metadata.Statistics.NullCount),
			); err != nil {
				fmt.Fprintf(os.Stderr, "encountered an error printing row in report: %v", err)
				os.Exit(1)
			}
		}
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

func buildRowsPerGroup(rowGroups []sawdust.RowGroup) string {
	var rowGroupString strings.Builder
	switch len(rowGroups) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("(%d)", rowGroups[0].NumRows)
	default:
		for i, group := range rowGroups {
			numRows := fmt.Sprintf("%d", group.NumRows)
			if i == 0 {
				rowGroupString.WriteString("(" + numRows + ", ")
			} else if i == len(rowGroups)-1 {
				rowGroupString.WriteString(numRows + ")")
			} else {
				rowGroupString.WriteString(numRows + ", ")
			}
		}
	}
	return rowGroupString.String()
}

func encodingNames(list []sawdust.Encoding) string {
	names := make([]string, len(list))
	for i, e := range list {
		names[i] = e.String()
	}
	return strings.Join(names, ",")
}
