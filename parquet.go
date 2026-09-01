package sawdust

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/bits"
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

func (f *File) decompress(p Page, codec CompressionCodec) ([]byte, error) {
	var levelsLen int64
	var compressed bool

	if p.Header.Type == DictionaryPage {
		levelsLen = 0
		compressed = codec != CodecUncompressed
	} else {
		h := p.Header.DataPageHeaderV2
		if h == nil {
			return nil, fmt.Errorf("no data page header available")
		}
		levelsLen = h.RepetitionLevelsByteLength + h.DefinitionLevelsByteLength
		compressed = h.IsCompressed
	}

	if levelsLen < 0 || int(levelsLen) > len(p.Data) {
		return nil, fmt.Errorf("malformed level length (%d)", levelsLen)
	}

	wantLen := int(p.Header.UncompressedPageSize - levelsLen)
	if wantLen < 0 {
		return nil, fmt.Errorf("uncompressed page size (%d) exceeds total levels length (%d)", p.Header.UncompressedPageSize, levelsLen)
	}
	payload := p.Data[levelsLen:]

	var out []byte
	if !compressed {
		out = payload
	} else {
		switch codec {
		case CodecZSTD:
			dec, err := f.zstdDecoder()
			if err != nil {
				return nil, err
			}
			out, err = dec.DecodeAll(payload, make([]byte, 0, wantLen))
			if err != nil {
				return nil, fmt.Errorf("unexpected error decoding payload: %w", err)
			}
		default:
			return nil, fmt.Errorf("codec %s is not supported", codec)
		}
	}

	if len(out) != wantLen {
		return nil, fmt.Errorf("expected %d bytes after decoding, but got %d", wantLen, len(out))
	}
	return out, nil
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

func (f *File) ReadColumn(col string) (ColumnValues, error) {
	root, err := BuildTree(f.Metadata.Schema)
	if err != nil {
		return nil, err
	}

	columns := Columns(root)
	colName := ""
	maxDefLevel := int32(-1)
	var colType PhysicalType
	for _, c := range columns {
		name := strings.Join(c.Path, ".")
		t := c.Element.Type

		if name == col {
			colName = name
			if t == nil {
				return nil, fmt.Errorf("no column type defined in schema for %q", col)
			}
			maxDefLevel = int32(c.MaxDefinitionLevel)
			colType = *t
			break
		}

	}

	if colName == "" {
		return nil, fmt.Errorf("%s not found in schema", col)
	}

	var chunks []ColumnChunk
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
			chunks = append(chunks, c)
		}
	}

	switch colType {
	case TypeInt64:
		out, err := collect(f, chunks, maxDefLevel, func(b []byte, _ int) ([]int64, error) { return decodePlainInt64(b) })
		if err != nil {
			return nil, err
		}
		return Int64Values(out), nil
	case TypeDouble:
		out, err := collect(f, chunks, maxDefLevel, func(b []byte, _ int) ([]float64, error) { return decodePlainDouble(b) })
		if err != nil {
			return nil, err
		}
		return DoubleValues(out), nil
	case TypeByteArray:
		out, err := collect(f, chunks, maxDefLevel, func(b []byte, _ int) ([][]byte, error) { return decodePlainByteArray(b) })
		if err != nil {
			return nil, err
		}
		return ByteArrayValues(out), nil
	case TypeBoolean:
		out, err := collect(f, chunks, maxDefLevel, func(b []byte, count int) ([]bool, error) { return decodePlainBoolean(b, count) })
		if err != nil {
			return nil, err
		}
		return BooleanValues(out), nil
	default:
		return nil, fmt.Errorf("unhandled type %s, unable to decode", colType)
	}
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

func collect[T any](f *File, chunks []ColumnChunk, maxDefLevel int32, decode func(b []byte, count int) ([]T, error)) ([]*T, error) {
	var out []*T
	for _, chunk := range chunks {
		pages, err := f.ReadPages(chunk)
		if err != nil {
			return nil, err
		}
		codec := chunk.Metadata.Codec
		colName := strings.Join(chunk.Metadata.PathInSchema, ".")
		var dictionary []T

		for _, page := range pages {
			if page.Header.Type == DictionaryPage {
				decompressed, err := f.decompress(page, codec)
				if err != nil {
					return nil, err
				}
				dictionary, err = decode(decompressed, int(page.Header.DictionaryPageHeader.NumValues))
				if err != nil {
					return nil, err
				}
				continue
			}

			pageHeader := page.Header.DataPageHeaderV2
			if pageHeader == nil {
				return nil, fmt.Errorf("page error: no data page header provided for %q", colName)
			}

			valueBytes, err := f.decompress(page, codec)
			if err != nil {
				return nil, fmt.Errorf("decompression error on %q: %w", colName, err)
			}

			if len(valueBytes) == 0 {
				return nil, fmt.Errorf("decompression returned 0 bytes")
			}

			repLen := int(pageHeader.RepetitionLevelsByteLength)
			defLen := int(pageHeader.DefinitionLevelsByteLength)
			defLevelBytes := page.Data[repLen : repLen+defLen]
			count := int(pageHeader.NumValues)
			storedCount := int(pageHeader.NumValues - pageHeader.NumNulls)
			bitWidth := bits.Len(uint(maxDefLevel))

			levels := make([]int32, count)
			if maxDefLevel > 0 {
				levels, err = decodeRLE(defLevelBytes, bitWidth, count)
				if err != nil {
					return nil, err
				}
			}

			var decodedBytes []T
			switch pageHeader.Encoding {
			case EncodingPlain:
				decodedBytes, err = decode(valueBytes, storedCount)
				if err != nil {
					return nil, err
				}
			case EncodingRLEDictionary:
				if dictionary == nil {
					return nil, fmt.Errorf("column %q: dictionary-encoded page with no dictionary", colName)
				}
				bitWidth := int(valueBytes[0])
				indices, err := decodeRLE(valueBytes[1:], bitWidth, storedCount)
				if err != nil {
					return nil, err
				}

				values := make([]T, 0, len(indices))
				for _, i := range indices {
					if i < 0 || int(i) >= len(dictionary) {
						return nil, fmt.Errorf("column %q: dictionary index %d is out of range (%d entries)", colName, i, len(dictionary))
					}
					values = append(values, dictionary[i])
				}
				decodedBytes = append(decodedBytes, values...)
			}

			expandedValues, err := applyDefinitionLevels(levels, decodedBytes, maxDefLevel)
			if err != nil {
				return nil, err
			}
			out = append(out, expandedValues...)
		}
	}
	return out, nil
}
