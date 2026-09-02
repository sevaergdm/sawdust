package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

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

	vals, err := file.ReadColumn(col)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	switch v := vals.(type) {
	case sawdust.Int64Values:
		err = printAll(os.Stdout, v, func(x int64) string { return strconv.FormatInt(x, 10) })
	case sawdust.DoubleValues:
		err = printAll(os.Stdout, v, func(x float64) string { return strconv.FormatFloat(x, 'g', -1, 64) })
	case sawdust.ByteArrayValues:
		err = printAll(os.Stdout, v, func(x []byte) string { return string(x) })
	case sawdust.BooleanValues:
		err = printAll(os.Stdout, v, func(x bool) string { return strconv.FormatBool(x) })
	case sawdust.TimestampValues:
		err = printAll(os.Stdout, v, func(x time.Time) string { return x.Format(time.RFC3339) })
	default:
		err = fmt.Errorf("unsupported type %s", v)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "encountered an error printing values: %v", err)
	}

}

func printAll[T any](w io.Writer, vals []*T, format func(T) string) error {
	for _, p := range vals {
		if p == nil {
			_, err := fmt.Fprintln(w, "")
			if err != nil {
				return err
			}
			continue
		}

		if _, err := fmt.Fprintln(w, format(*p)); err != nil {
			return err
		}
	}
	return nil
}
