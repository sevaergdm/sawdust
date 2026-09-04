package main

import (
	"encoding/hex"
	"encoding/json"
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

	if fs.NArg() != 2 && fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: sawdust cat <file> [column]\n")
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

	if col == "" {
		if err := writeRows(file, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "could not write rows for %q: %v\n", path, err)
			os.Exit(1)
		}
		return
	}

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
		err = fmt.Errorf("unsupported type %T", v)
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

func writeRows(f *sawdust.File, w io.Writer) error {
	colMap := make(map[string][]any)
	numRows := int(f.Metadata.NumRows)
	root, err := sawdust.BuildTree(f.Metadata.Schema)
	if err != nil {
		return err
	}

	columns := sawdust.Columns(root)
	for _, c := range columns {
		name := strings.Join(c.Path, ".")
		cv, err := f.ReadColumn(name)
		if err != nil {
			return fmt.Errorf("error reading %s: %w", name, err)
		}

		nv, err := normalizeValues(cv)
		if err != nil {
			return fmt.Errorf("error normalizing %s: %w", name, err)
		}

		if len(nv) != numRows {
			return fmt.Errorf("%s: found %d column values, but had %d rows", name, len(nv), numRows)
		}
		colMap[name] = nv
	}

	for i := 0; i < int(f.Metadata.NumRows); i++ {
		rec := make(map[string]any, len(colMap))
		for name, vals := range colMap {
			rec[name] = vals[i]
		}

		enc := json.NewEncoder(w)
		if err := enc.Encode(rec); err != nil {
			return err
		}
	}
	return nil
}

func normalizeValues(c sawdust.ColumnValues) ([]any, error) {
	if lv, ok := c.(sawdust.ListValues); ok {
		inner, err := normalizeValues(lv.Elements)
		if err != nil {
			return nil, err
		}
		out := make([]any, 0, len(lv.Offsets)-1)
		for i := 0; i+1 < len(lv.Offsets); i++ {
			row := make([]any, 0, lv.Offsets[i+1]-lv.Offsets[i])
			row = append(row, inner[lv.Offsets[i]:lv.Offsets[i+1]]...)
			out = append(out, row)
		}
		return out, nil
	}

	switch v := c.(type) {
	case sawdust.Int64Values:
		return boxed(v, func(x int64) any { return x }), nil
	case sawdust.DoubleValues:
		return boxed(v, func(x float64) any { return x }), nil
	case sawdust.BooleanValues:
		return boxed(v, func(x bool) any { return x }), nil
	case sawdust.StringValues:
		return boxed(v, func(x string) any { return x }), nil
	case sawdust.TimestampValues:
		return boxed(v, func(x time.Time) any { return x }), nil
	case sawdust.ByteArrayValues:
		return boxed(v, func(x []byte) any { return hex.EncodeToString(x) }), nil
	default:
		return nil, fmt.Errorf("unsupported column type %T", c)
	}
}

func boxed[T any](vals []*T, conv func(T) any) []any {
	out := make([]any, 0, len(vals))
	for _, p := range vals {
		if p == nil {
			out = append(out, nil)
			continue
		}
		out = append(out, conv(*p))
	}
	return out
}
