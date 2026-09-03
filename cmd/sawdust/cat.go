package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
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

	var offsets []int
	if lv, ok := vals.(sawdust.ListValues); ok {
		offsets = lv.Offsets
		vals = lv.Elements
	}

	switch v := vals.(type) {
	case sawdust.Int64Values:
		err = printValues(os.Stdout, v, offsets, func(x int64) string { return strconv.FormatInt(x, 10) })
	case sawdust.DoubleValues:
		err = printValues(os.Stdout, v, offsets, func(x float64) string { return strconv.FormatFloat(x, 'g', -1, 64) })
	case sawdust.StringValues:
		err = printValues(os.Stdout, v, offsets, func(x string) string { return x })
	case sawdust.ByteArrayValues:
		err = printValues(os.Stdout, v, offsets, func(x []byte) string { return fmt.Sprintf("%x", x) })
	case sawdust.BooleanValues:
		err = printValues(os.Stdout, v, offsets, func(x bool) string { return strconv.FormatBool(x) })
	case sawdust.TimestampValues:
		err = printValues(os.Stdout, v, offsets, func(x time.Time) string { return x.Format(time.RFC3339) })
	default:
		err = fmt.Errorf("unsupported type %s", v)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "encountered an error printing values: %v", err)
	}

}

func printValues[T any](w io.Writer, vals []*T, offsets []int, format func(T) string) error {
	if offsets == nil {
		for _, p := range vals {
			if p == nil {
				_, err := fmt.Fprintln(w, "NULL")
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

	for i := 0; i+1 < len(offsets); i++ {
		parts := make([]string, 0, offsets[i+1]-offsets[i])
		for _, p := range vals[offsets[i]:offsets[i+1]] {
			if p == nil {
				parts = append(parts, "NULL")
			} else {
				parts = append(parts, format(*p))
			}
		}
		if _, err := fmt.Fprintf(w, "[%s]\n", strings.Join(parts, ", ")); err != nil {
			return err
		}
	}
	return nil
}
