package sawdust

import (
	"fmt"
	"math/bits"
	"strings"
)

func (f *File) ReadColumn(col string) (ColumnValues, error) {
	root, err := BuildTree(f.Metadata.Schema)
	if err != nil {
		return nil, err
	}

	columns := Columns(root)
	colName := ""
	maxDefLevel := int64(-1)
	maxRepLevel := int64(0)
	var logicalType LogicalType
	var convertedType *ConvertedType
	var colType PhysicalType
	for _, c := range columns {
		name := strings.Join(c.Path, ".")
		t := c.Element.Type

		if name == col {
			colName = name
			if t == nil {
				return nil, fmt.Errorf("no column type defined in schema for %q", col)
			}
			maxDefLevel = int64(c.MaxDefinitionLevel)
			maxRepLevel = int64(c.MaxRepetitionLevel)
			logicalType = c.Element.LogicalType
			convertedType = c.Element.ConvertedType
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

	var flat ColumnValues
	var offsets []int
	switch colType {
	case TypeInt64:
		out, off, err := collect(f, chunks, maxDefLevel, maxRepLevel, func(b []byte, _ Encoding, _ int) ([]int64, error) { return decodePlainInt64(b) })
		if err != nil {
			return nil, err
		}
		offsets = off
		flat = Int64Values(out)
		if ts, ok := logicalType.(TimestampType); ok {
			if !ts.IsAdjustedToUTC {
				return nil, fmt.Errorf("column %q: timestamps without UTC adjustment are not supported", col)
			}
			timeOut, err := toTimes(out, ts)
			if err != nil {
				return nil, fmt.Errorf("column %q: %w", col, err)
			}
			flat = timeOut
		}
	case TypeDouble:
		out, off, err := collect(f, chunks, maxDefLevel, maxRepLevel, func(b []byte, _ Encoding, _ int) ([]float64, error) { return decodePlainDouble(b) })
		if err != nil {
			return nil, err
		}
		offsets = off
		flat = DoubleValues(out)
	case TypeByteArray:
		out, off, err := collect(f, chunks, maxDefLevel, maxRepLevel, func(b []byte, enc Encoding, _ int) ([][]byte, error) {
			switch enc {
			case EncodingPlain:
				return decodePlainByteArray(b)
			case EncodingDeltaLengthByteArray:
				return decodeDeltaLengthByteArray(b)
			default:
				return nil, fmt.Errorf("unsupported encoding  %s for byte_array", enc)
			}
		})
		if err != nil {
			return nil, err
		}
		isText := false
		if _, ok := logicalType.(StringType); ok {
			isText = true
		} else if logicalType == nil && convertedType != nil && *convertedType == ConvertedUTF8 {
			isText = true
		}
		offsets = off

		if isText {
			stringOut, err := toStrings(out)
			if err != nil {
				return nil, err
			}
			flat = stringOut
		} else {
			flat = ByteArrayValues(out)
		}
	case TypeBoolean:
		out, off, err := collect(f, chunks, maxDefLevel, maxRepLevel, func(b []byte, _ Encoding, count int) ([]bool, error) { return decodePlainBoolean(b, count) })
		if err != nil {
			return nil, err
		}
		offsets = off
		flat = BooleanValues(out)
	default:
		return nil, fmt.Errorf("unhandled type %s, unable to decode", colType)
	}

	if maxRepLevel > 0 {
		return ListValues{Elements: flat, Offsets: offsets}, nil
	}
	return flat, nil
}

func collect[T any](f *File, chunks []ColumnChunk, maxDefLevel, maxRepLevel int64, decode func(b []byte, enc Encoding, count int) ([]T, error)) ([]*T, []int, error) {
	var out []*T
	var offsets []int
	for _, chunk := range chunks {
		pages, err := f.ReadPages(chunk)
		if err != nil {
			return nil, nil, err
		}
		codec := chunk.Metadata.Codec
		colName := strings.Join(chunk.Metadata.PathInSchema, ".")
		var dictionary []T

		for _, page := range pages {
			if page.Header.Type == DictionaryPage {
				decompressed, err := f.decompress(page, codec)
				if err != nil {
					return nil, nil, err
				}
				dictionary, err = decode(decompressed, page.Header.DictionaryPageHeader.Encoding, int(page.Header.DictionaryPageHeader.NumValues))
				if err != nil {
					return nil, nil, err
				}
				continue
			}

			pageHeader := page.Header.DataPageHeaderV2
			if pageHeader == nil {
				return nil, nil, fmt.Errorf("page error: no data page header provided for %q", colName)
			}

			valueBytes, err := f.decompress(page, codec)
			if err != nil {
				return nil, nil, fmt.Errorf("decompression error on %q: %w", colName, err)
			}

			repLen := int(pageHeader.RepetitionLevelsByteLength)
			defLen := int(pageHeader.DefinitionLevelsByteLength)
			repLevelBytes := page.Data[:repLen]
			defLevelBytes := page.Data[repLen : repLen+defLen]
			repCount := int(pageHeader.NumValues)
			defCount := int(pageHeader.NumValues)
			storedCount := int(pageHeader.NumValues - pageHeader.NumNulls)
			repBitWidth := bits.Len(uint(maxRepLevel))
			defBitWidth := bits.Len(uint(maxDefLevel))

			defLevels := make([]int64, defCount)
			if maxDefLevel > 0 {
				defLevels, err = decodeRLE(defLevelBytes, defBitWidth, defCount)
				if err != nil {
					return nil, nil, err
				}
			}

			repLevels := make([]int64, repCount)
			if maxRepLevel > 0 {
				repLevels, err = decodeRLE(repLevelBytes, repBitWidth, repCount)
				if err != nil {
					return nil, nil, err
				}
			}

			var decodedBytes []T
			switch pageHeader.Encoding {
			case EncodingRLEDictionary:
				if storedCount == 0 {
					break
				}
				if dictionary == nil {
					return nil, nil, fmt.Errorf("column %q: dictionary-encoded page with no dictionary", colName)
				}
				bitWidth := int(valueBytes[0])
				indices, err := decodeRLE(valueBytes[1:], bitWidth, storedCount)
				if err != nil {
					return nil, nil, err
				}

				values := make([]T, 0, len(indices))
				for _, i := range indices {
					if i < 0 || int(i) >= len(dictionary) {
						return nil, nil, fmt.Errorf("column %q: dictionary index %d is out of range (%d entries)", colName, i, len(dictionary))
					}
					values = append(values, dictionary[i])
				}
				decodedBytes = append(decodedBytes, values...)
			default:
				decodedBytes, err = decode(valueBytes, pageHeader.Encoding, storedCount)
				if err != nil {
					return nil, nil, err
				}
			}

			expandedValues, chunkOffsets, err := applyLevels(repLevels, defLevels, decodedBytes, maxDefLevel, maxRepLevel, int(pageHeader.NumRows))
			if err != nil {
				return nil, nil, err
			}

			base := len(out)
			if len(offsets) == 0 {
				offsets = append(offsets, chunkOffsets...)
			} else {
				for _, o := range chunkOffsets[1:] {
					offsets = append(offsets, base+o)
				}
			}
			out = append(out, expandedValues...)
		}
	}
	return out, offsets, nil
}
