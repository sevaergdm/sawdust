package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/sevaergdm/sawdust"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "footer":
		cmdFooter(os.Args[2:])
	case "schema":
		cmdSchema(os.Args[2:])
	case "stat":
		cmdStat(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "error: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}

}

func usage() {
	fmt.Fprint(os.Stderr, `usage: sawdust <command> [arguments]

Commands:
  footer <file>   print the file's byte layout
  schema <file>   print the schema tree
	stat 	 <file> 	print file stats
`)
}

func cmdStat(args []string) {
	fs := flag.NewFlagSet("stat", flag.ExitOnError)
	func() { _ = fs.Parse(args) }()

	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: sawdust stat <file>\n")
		os.Exit(2)
	}

	path := args[0]
	file, err := sawdust.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not read %q: %v\n", path, err)
		os.Exit(1)
	}

	rowsPerGroup := buildRowsPerGroup(file.Metadata.RowGroups)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if _, err := fmt.Fprintf(w, "rows: %d  row groups: %d %s file: %d bytes\n", file.Metadata.NumRows, len(file.Metadata.RowGroups), rowsPerGroup, file.Size); err != nil {
		fmt.Fprintf(os.Stderr, "encountered an error printing file summary: %v", err)
		os.Exit(1)
	}

	fmt.Println()

	if _, err := fmt.Fprintln(w, "group\tcolumn\ttype\tcodec\tencodings\tvalues\tcomp\tuncomp\tratio\tnulls"); err != nil {
		fmt.Fprintf(os.Stderr, "encountered an error printing report header: %v", err)
		os.Exit(1)
	}

	totalCompressedBytes := make(map[string]int64)
	var sortedColumns []columnBytes
	for i, group := range file.Metadata.RowGroups {
		for _, c := range group.Columns {
			metadata := c.Metadata
			totalCompressedBytes[strings.Join(metadata.PathInSchema, ".")] += metadata.TotalCompressedSize

			ratio := "-"
			if metadata.TotalCompressedSize > 0 {
				ratio = fmt.Sprintf("%.2f", float64(metadata.TotalUncompressedSize)/float64(metadata.TotalCompressedSize))
			}

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

	for k, v := range totalCompressedBytes {
		sortedColumns = append(sortedColumns, columnBytes{name: k, compressedBytes: v})
	}

	sort.Slice(sortedColumns, func(i, j int) bool {
		return sortedColumns[i].compressedBytes > sortedColumns[j].compressedBytes
	})

	if _, err := fmt.Fprintln(w, ""); err != nil {
		fmt.Fprintf(os.Stderr, "encountered an error printing blank line: %v", err)
		os.Exit(1)
	}

	if _, err := fmt.Fprintln(w, "by compressed bytes:"); err != nil {
		fmt.Fprintf(os.Stderr, "encountered an error printing compressed bytes list header: %v", err)
		os.Exit(1)
	}

	for _, col := range sortedColumns {
		if _, err := fmt.Fprintf(w, "\t%s\t%d\n", col.name, col.compressedBytes); err != nil {
			fmt.Fprintf(os.Stderr, "encountered an error printing sorted column row: %v", err)
			os.Exit(1)
		}
	}

	if err := w.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "encountered an error printing column report: %v", err)
		os.Exit(1)
	}

}

func cmdFooter(args []string) {
	fs := flag.NewFlagSet("footer", flag.ExitOnError)
	func() { _ = fs.Parse(args) }()

	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: sawdust footer <file>\n")
		os.Exit(2)
	}

	path := args[0]
	file, err := sawdust.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not read %q: %v\n", path, err)
		os.Exit(1)
	}

	fmt.Printf("file size: %d bytes\n", file.Size)
	fmt.Printf("footer length: %d bytes\n", file.Footer.Length)
	fmt.Printf("metadata start position: %d\n", file.Footer.Start)
	fmt.Printf("num rows: %d\n", file.Metadata.NumRows)
	fmt.Printf("version;: %d\n", file.Metadata.Version)
	fmt.Printf("created by: %q\n", file.Metadata.CreatedByOrEmpty())
}

func cmdSchema(args []string) {
	fs := flag.NewFlagSet("schema", flag.ExitOnError)
	func() { _ = fs.Parse(args) }()

	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: sawdust schema <file>\n")
		os.Exit(2)
	}

	path := fs.Arg(0)
	file, err := sawdust.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not read %q: %v\n", path, err)
		os.Exit(1)
	}

	root, err := sawdust.BuildTree(file.Metadata.Schema)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not build tree: %v\n", err)
		os.Exit(1)
	}

	byPath := map[string]sawdust.Column{}
	for _, c := range sawdust.Columns(root) {
		byPath[strings.Join(c.Path, ".")] = c
	}

	fmt.Printf("(%s)\n", root.Element.Name)
	for _, c := range root.Children {
		printNode(c, 1, nil, byPath)
	}
}

func printNode(n sawdust.SchemaNode, depth int, path []string, byPath map[string]sawdust.Column) {
	path = append(path, n.Element.Name)
	indent := strings.Repeat("  ", depth)

	if len(n.Children) == 0 {
		col, ok := byPath[strings.Join(path, ".")]
		if !ok {
			fmt.Printf("column %s does not exist in schema", strings.Join(path, "."))
			return
		}
		fmt.Printf("%s%s %v %v  def=%d rep=%d\n", indent, n.Element.Name, n.Element.Type, n.Element.RepetitionType, col.MaxDefinitionLevel, col.MaxRepetitionLevel)
		return
	}

	fmt.Printf("%s%s %v\n", indent, n.Element.Name, n.Element.RepetitionType)
	for _, child := range n.Children {
		printNode(child, depth+1, path, byPath)
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

func orDash(p *int64) string {
	if p == nil {
		return "-"
	}
	return strconv.FormatInt(*p, 10)
}

func encodingNames(list []sawdust.Encoding) string {
	names := make([]string, len(list))
	for i, e := range list {
		names[i] = e.String()
	}
	return strings.Join(names, ",")
}

type columnBytes struct {
	name            string
	compressedBytes int64
}
