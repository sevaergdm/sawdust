package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/parquet-go/parquet-go"
)

var category = []string{"foo", "bar", "baz", "buzz"}

type row struct {
	RowNumber     int64     `parquet:"row_number"`
	EvenRowNumber *int64    `parquet:"even_row_number,optional"`
	RandId        string    `parquet:"rand_id"`
	OptRandId     *string   `parquet:"opt_rand_id,optional"`
	Category      string    `parquet:"category"`
	RandFloat     float64   `parquet:"rand_float"`
	Ts            time.Time `parquet:"ts,timestamp(microsecond)"`
	IsOdd         bool      `parquet:"is_odd"`
}

type inner struct {
	A int64  `parquet:"a"`
	B string `parquet:"b"`
}

type nestedRow struct {
	ID    int64    `parquet:"id"`
	In    inner    `parquet:"inner"`
	OptIn *inner   `parquet:"opt_in,optional"`
	Tags  []string `parquet:"tags"`
}

type rawRow struct {
	Text string `parquet:"text"`
	Raw  []byte `parquet:"raw"`
}

// Command genfix generates the Parquet fixtures in testdata/.
//
// The committed fixtures were produced with
// github.com/parquet-go/parquet-go v0.32.0. Regenerating with a different
// version may change the bytes even if this generator is unchanged.
func main() {
	outDir := flag.String("out", "", "output directory")
	numRows := flag.Int("num_rows", 100, "number of rows to write to the parquet file")
	fileName := flag.String("file_name", "", "the name of the file")
	rowGroupSize := flag.Int64("row_group_size", 0, "the size of the row groups, uses default if not specified")
	kind := flag.String("kind", "row", "the kind of row to write")
	compression := flag.String("compression", "", "the compression to apply, none if not supplied")
	pageBufferSize := flag.Int("page_buffer_size", 0, "page buffer size in bytes")
	nulls := flag.String("null", "", "whether optional fields should be all nulls ('all_nulls') or no nulls ('no_nulls')")
	encoding := flag.String("encoding", "", "type of encoding to use for columns. Either 'plain' or 'dict'")
	flag.Parse()
	if *outDir == "" {
		fmt.Fprintf(os.Stderr, "error: no output directory specified\n")
		flag.Usage()
		os.Exit(2)
	}

	if *fileName == "" {
		fmt.Fprintf(os.Stderr, "error: no output file name specified\n")
		flag.Usage()
		os.Exit(2)
	}

	switch *nulls {
	case "", "all_nulls", "no_nulls":
	default:
		fmt.Fprintf(os.Stderr, "error: unknown -null value %q, must be all_nulls, no_nulls, or omitted\n", *nulls)
		os.Exit(2)
	}

	if err := os.MkdirAll(*outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error: encountered an error creating directory %s: %v\n", *outDir, err)
		os.Exit(1)
	}

	path := filepath.Join(*outDir, *fileName+".parquet")
	var err error
	switch *kind {
	case "row":
		err = writeRows(path, buildRows(*numRows, *nulls), *rowGroupSize, *compression, *pageBufferSize, *encoding)
	case "nested":
		err = writeRows(path, buildNestedRows(*numRows), *rowGroupSize, *compression, *pageBufferSize, *encoding)
	case "raw":
		err = writeRows(path, buildRawRows(*numRows), *rowGroupSize, *compression, *pageBufferSize, *encoding)
	default:
		err = fmt.Errorf("unknown kind: %s. Only row, nested, and raw accepted", *kind)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

}

func writeRows[T any](path string, rows []T, rowGroupSize int64, compression string, pageBufferSize int, encoding string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create parquet file '%s': %v", path, err)
	}
	defer func() { _ = f.Close() }()

	var options []parquet.WriterOption
	if rowGroupSize > 0 {
		options = append(options, parquet.MaxRowsPerRowGroup(rowGroupSize))
	}

	switch strings.ToLower(compression) {
	case "":
	case "zstd":
		options = append(options, parquet.Compression(&parquet.Zstd))
	case "gzip":
		options = append(options, parquet.Compression(&parquet.Gzip))
	case "snappy":
		options = append(options, parquet.Compression(&parquet.Snappy))
	case "lz4raw":
		options = append(options, parquet.Compression(&parquet.Lz4Raw))
	case "brotli":
		options = append(options, parquet.Compression(&parquet.Brotli))
	default:
		return fmt.Errorf("unknown compression codec %s provided, possible values are: zstd, gzip, snappy, lz4raw, brotli", compression)
	}

	if pageBufferSize > 0 {
		options = append(options, parquet.PageBufferSize(pageBufferSize))
	}

	switch strings.ToLower(encoding) {
	case "":
	case "plain":
		options = append(options, parquet.DefaultEncoding(&parquet.Plain))
	case "dict":
		options = append(options, parquet.DefaultEncoding(&parquet.RLEDictionary))
	default:
		return fmt.Errorf("unknown encoding %s provided, supported values are: plain, dict", encoding)
	}

	writer := parquet.NewGenericWriter[T](f, options...)
	_, err = writer.Write(rows)
	if err != nil {
		return fmt.Errorf("could not write to '%s': %v", path, err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("encountered an error closing parquet writer: %v", err)
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("encountered an error syncing to disk: %v", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("encountered an error closing file: %v", err)
	}
	return nil
}

func buildRows(n int, nulls string) []row {
	rows := make([]row, 0, n)
	currentRowNumber := int64(0)
	randGen := rand.New(rand.NewSource(42))
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for n > 0 {
		currentRowNumber++
		n--
		var r row
		setEven := nulls == "no_nulls" || (nulls == "" && currentRowNumber%2 == 0)
		setOpt := nulls == "no_nulls" || (nulls == "" && currentRowNumber%3 == 0)
		r.RowNumber = currentRowNumber
		r.RandId = fmt.Sprintf("id-%04d", currentRowNumber)
		r.IsOdd = true

		if setEven {
			row := currentRowNumber
			r.EvenRowNumber = &row
		}

		if setOpt {
			randId := fmt.Sprintf("opt-%04d", currentRowNumber)
			r.OptRandId = &randId
		}

		if currentRowNumber%2 == 0 {
			r.IsOdd = false
		}

		r.Category = category[randGen.Intn(4)]
		r.RandFloat = randGen.Float64()
		r.Ts = base.Add(time.Duration(currentRowNumber) * time.Second)

		rows = append(rows, r)
	}
	return rows
}

func buildNestedRows(n int) []nestedRow {
	rows := make([]nestedRow, 0, n)
	for i := 1; i <= n; i++ {
		r := nestedRow{
			ID: int64(i),
			In: inner{A: int64(i * 10), B: fmt.Sprintf("in-%04d", i)},
		}

		if i%2 == 0 {
			r.OptIn = &inner{A: int64(i * 100), B: fmt.Sprintf("opt-%04d", i)}
		}
		for j := 0; j < i%3; j++ {
			r.Tags = append(r.Tags, fmt.Sprintf("tag-%d-%d", i, j))
		}
		rows = append(rows, r)
	}
	return rows
}

func buildRawRows(n int) []rawRow {
	rows := make([]rawRow, 0, n)
	for i := 1; i <= n; i++ {
		var r rawRow
		r.Text = fmt.Sprintf("txt-%04d", i)
		r.Raw = []byte{0x00, 0x1b, byte(i), 0xff}
		if i%5 == 0 {
			r.Raw = []byte{}
		}
		rows = append(rows, r)
	}
	return rows
}
