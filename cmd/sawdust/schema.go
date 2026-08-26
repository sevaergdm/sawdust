package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sevaergdm/sawdust"
)

func cmdSchema(args []string) {
	fs := flag.NewFlagSet("schema", flag.ExitOnError)
	_ = fs.Parse(args)

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
