package sawdust

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

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

func (f *File) Close() error {
	if f.zstdDec != nil {
		f.zstdDec.Close()
	}
	if f.closer == nil {
		return nil
	}
	return f.closer.Close()
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
	if chunk.Metadata.DictionaryPageOffset != nil {
		offset := *chunk.Metadata.DictionaryPageOffset
		if _, err := f.reader.ReadAt(buf, offset); err != nil {
			return nil, fmt.Errorf("encountered error reading file at %d: %w", *chunk.Metadata.DictionaryPageOffset, err)
		}
	} else {
		if _, err := f.reader.ReadAt(buf, chunk.Metadata.DataPageOffset); err != nil {
			return nil, fmt.Errorf("encountered error reading file at %d: %w", chunk.Metadata.DataPageOffset, err)
		}
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
		if page.Header.Type == DictionaryPage {
			continue
		}
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
