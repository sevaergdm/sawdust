package sawdust

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/sevaergdm/sawdust/internal/thrift"
)

type Footer struct {
	Length int64
	Start  int64
}

type File struct {
	reader   io.ReaderAt
	closer   io.Closer
	Size     int64
	Footer   Footer
	Metadata FileMetadata
	zstdDec  *zstd.Decoder
}

func (f *File) zstdDecoder() (*zstd.Decoder, error) {
	if f.zstdDec == nil {
		dec, err := zstd.NewReader(nil)
		if err != nil {
			return nil, err
		}
		f.zstdDec = dec
	}
	return f.zstdDec, nil
}

func (f *File) Close() error {
	if f.zstdDec != nil {
		f.zstdDec.Close()
	}
	if f.closer == nil {
		return nil
	}
	return f.closer.Close()
}

func (f *File) ReadInt64Column(col string) ([]int64, error) {
	var output []int64
	dec, err := f.zstdDecoder()
	if err != nil {
		return nil, err
	}

	root, err := BuildTree(f.Metadata.Schema)
	if err != nil {
		return nil, err
	}

	columns := Columns(root)
	colName := ""
	var colType PhysicalType
	for _, c := range columns {
		name := strings.Join(c.Path, ".")
		t := c.Element.Type

		if name == col {
			colName = name
			if t == nil {
				return nil, fmt.Errorf("no column type defined in schema for %q", col)
			}

			if *t != TypeInt64 {
				return nil, fmt.Errorf("column %q is not an int64 and not yet supported", col)
			}
			colType = *t
			break
		}
	}

	if colName == "" {
		return nil, fmt.Errorf("%s not found in schema", col)
	}

	for _, group := range f.Metadata.RowGroups {
		for _, c := range group.Columns {
			if c.Metadata == nil {
				continue
			}

			if strings.Join(c.Metadata.PathInSchema, ".") != col {
				continue
			}

			if c.Metadata.Type != colType {
				return nil, fmt.Errorf("malformed column %q: type %s does not match schema defined type %s", col, c.Metadata.Type, colType)
			}

			pages, err := f.ReadPages(c)
			if err != nil {
				return nil, fmt.Errorf("page read error for %q: %w", col, err)
			}

			for _, page := range pages {
				pageHeader := page.Header.DataPageHeaderV2
				if pageHeader == nil {
					return nil, fmt.Errorf("page error: no data page header provided for %q", col)
				}

				if pageHeader.Encoding != EncodingPlain {
					return nil, fmt.Errorf("page error: encoding for column %q (%s) must be PLAIN", col, pageHeader.Encoding)
				}

				decompressed, err := page.decompressZstd(dec)
				if err != nil {
					return nil, fmt.Errorf("decompression error on %q: %w", col, err)
				}

				vals, err := decodePlainInt64(decompressed)
				if err != nil {
					return nil, fmt.Errorf("decoding error on %q: %w", col, err)
				}
				output = append(output, vals...)
			}
		}
	}
	return output, nil
}

func (f *File) ReadPages(chunk ColumnChunk) ([]Page, error) {
	if chunk.Metadata == nil {
		return nil, fmt.Errorf("no page data to read")
	}

	if chunk.Metadata.TotalCompressedSize < 0 || chunk.Metadata.TotalCompressedSize > f.Size {
		return nil, fmt.Errorf("malformed payload size (%d)", chunk.Metadata.TotalCompressedSize)
	}

	var pages []Page
	buf := make([]byte, chunk.Metadata.TotalCompressedSize)
	if _, err := f.reader.ReadAt(buf, chunk.Metadata.DataPageOffset); err != nil {
		return nil, fmt.Errorf("encountered error reading file at %d: %w", chunk.Metadata.DataPageOffset, err)
	}

	cursor := 0
	for cursor < int(chunk.Metadata.TotalCompressedSize) {
		d := thrift.NewDecoder(buf[cursor:])

		pageHeader, err := readPageHeader(d)
		if err != nil {
			return nil, err
		}

		headerLen := d.Pos()
		payloadStart := cursor + headerLen
		payloadEnd := payloadStart + int(pageHeader.CompressedPageSize)

		if payloadEnd < payloadStart || payloadEnd > len(buf) {
			return nil, fmt.Errorf("malformed payload end position (%d)", payloadEnd)
		}

		pages = append(pages, Page{Header: pageHeader, Data: buf[payloadStart:payloadEnd]})
		cursor = payloadEnd
	}

	totalValues := int64(0)
	for _, page := range pages {
		totalValues += page.Header.ValueCount()
	}

	if totalValues != chunk.Metadata.NumValues {
		return nil, fmt.Errorf("sum of page values (%d) does not match the total values in the column (%d)", totalValues, chunk.Metadata.NumValues)
	}

	return pages, nil
}

func ReadFooter(f io.ReaderAt, size int64) (Footer, error) {
	if size < 12 {
		return Footer{}, fmt.Errorf("malformed file, expected at least 12 bytes, but got %d", size)
	}

	head := make([]byte, 4)
	tail := make([]byte, 8)

	if _, err := f.ReadAt(head, 0); err != nil {
		return Footer{}, fmt.Errorf("unable to read first 4 bytes: %w", err)
	}

	if _, err := f.ReadAt(tail, size-8); err != nil {
		return Footer{}, fmt.Errorf("unable to read tail: %w", err)
	}

	if string(head) != "PAR1" || string(tail[4:8]) != "PAR1" {
		return Footer{}, fmt.Errorf("expected 'PAR1' as head and tail of file, but got: %q and %q", head, tail[4:8])
	}

	footerLength := int64(binary.LittleEndian.Uint32(tail[:4]))
	metadataStart := size - 8 - footerLength
	if metadataStart < 4 {
		return Footer{}, fmt.Errorf("metadata start position should be at least 4, got: %d", metadataStart)
	}

	return Footer{Length: footerLength, Start: metadataStart}, nil
}

func OpenFile(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("encountered an error opening file %q: %w", path, err)
	}

	fStat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("could not stat %q: %w", path, err)
	}

	size := fStat.Size()

	footer, err := ReadFooter(f, size)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("encountered an error reading footer in %q: %v", path, err)
	}

	footerBytes := make([]byte, footer.Length)
	if _, err := f.ReadAt(footerBytes, footer.Start); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("encountered an error reading from offset in %q: %v", path, err)
	}

	fileMetadata, err := ReadFileMetadata(footerBytes)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("encountered an error fetching file metadata in %q: %v", path, err)
	}
	return &File{reader: f, closer: f, Size: size, Footer: footer, Metadata: fileMetadata}, nil
}
