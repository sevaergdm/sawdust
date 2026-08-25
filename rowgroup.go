package sawdust

import (
	"fmt"

	"github.com/sevaergdm/sawdust/internal/thrift"
)

type requiredField struct {
	id   int64
	name string
}

type RowGroup struct {
	Columns             []ColumnChunk
	TotalByteSize       int64
	NumRows             int64
	FileOffset          *int64
	TotalCompressedSize *int64
	Ordinal             *int64
}

var rowGroupRequired = []requiredField{
	{id: 1, name: "columns"},
	{id: 2, name: "total_byte_size"},
	{id: 3, name: "num_rows"},
}

type ColumnChunk struct {
	FilePath   *string
	FileOffset int64 // deprecated, expect 0 value from most writers
	Metadata   *ColumnMetadata
}

var columnChunkRequired = []requiredField{
	{id: 2, name: "file_offset"},
}

type ColumnMetadata struct {
	Type                  PhysicalType
	Encodings             []Encoding
	PathInSchema          []string
	Codec                 CompressionCodec
	NumValues             int64
	TotalUncompressedSize int64
	TotalCompressedSize   int64
	DataPageOffset        int64
	DictionaryPageOffset  *int64
	Statistics            *Statistics
}

var columnMetadataRequired = []requiredField{
	{id: 1, name: "type"},
	{id: 2, name: "encodings"},
	{id: 3, name: "path_in_schema"},
	{id: 4, name: "codec"},
	{id: 5, name: "num_values"},
	{id: 6, name: "total_uncompressed_size"},
	{id: 7, name: "total_compressed_size"},
	{id: 9, name: "data_page_offset"},
}

type Statistics struct {
	NullCount     *int64
	DistinctCount *int64
	MaxValue      []byte
	MinValue      []byte
}

func readRowGroup(d *thrift.Decoder) (RowGroup, error) {
	var lastFieldID int64
	var rowGroup RowGroup
	seen := map[int64]bool{}

	for {
		fieldID, fieldType, err := d.FieldHeader(lastFieldID)
		if err != nil {
			return RowGroup{}, fmt.Errorf("unable to parse field header: %w", err)
		}

		if fieldType == thrift.TypeStop {
			break
		}

		switch fieldID {
		case 1:
			count, elemType, err := d.ListHeader()
			if err != nil {
				return RowGroup{}, fmt.Errorf("field %d: %w", fieldID, err)
			}

			if elemType != 12 {
				return RowGroup{}, fmt.Errorf("field %d: column chunk list holds type %d, but want struct", fieldID, elemType)
			}

			for range count {
				chunk, err := readColumnChunk(d)
				if err != nil {
					return RowGroup{}, fmt.Errorf("field %d: %w", fieldID, err)
				}
				rowGroup.Columns = append(rowGroup.Columns, chunk)
			}
		case 2:
			v, err := d.Int64()
			if err != nil {
				return RowGroup{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			rowGroup.TotalByteSize = v
		case 3:
			v, err := d.Int64()
			if err != nil {
				return RowGroup{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			rowGroup.NumRows = v
		case 5:
			v, err := d.Int64()
			if err != nil {
				return RowGroup{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			rowGroup.FileOffset = ptr(v)
		case 6:
			v, err := d.Int64()
			if err != nil {
				return RowGroup{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			rowGroup.TotalCompressedSize = ptr(v)
		case 7:
			v, err := d.Int64()
			if err != nil {
				return RowGroup{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			rowGroup.Ordinal = ptr(v)
		default:
			err := d.Skip(fieldType)
			if err != nil {
				return RowGroup{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
		}
		seen[fieldID] = true
		lastFieldID = fieldID
	}

	if err := checkRequired(seen, rowGroupRequired); err != nil {
		return RowGroup{}, err
	}
	return rowGroup, nil
}

func readColumnChunk(d *thrift.Decoder) (ColumnChunk, error) {
	var lastFieldID int64
	var chunk ColumnChunk
	seen := map[int64]bool{}

	for {
		fieldID, fieldType, err := d.FieldHeader(lastFieldID)
		if err != nil {
			return ColumnChunk{}, fmt.Errorf("unable to parse field header: %w", err)
		}

		if fieldType == thrift.TypeStop {
			break
		}

		switch fieldID {
		case 1:
			v, err := d.Text()
			if err != nil {
				return ColumnChunk{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			chunk.FilePath = ptr(v)
		case 2:
			v, err := d.Int64()
			if err != nil {
				return ColumnChunk{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			chunk.FileOffset = v
		case 3:
			metadata, err := readColumnMetadata(d)
			if err != nil {
				return ColumnChunk{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			chunk.Metadata = ptr(metadata)
		default:
			err := d.Skip(fieldType)
			if err != nil {
				return ColumnChunk{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
		}
		seen[fieldID] = true
		lastFieldID = fieldID
	}

	if err := checkRequired(seen, columnChunkRequired); err != nil {
		return ColumnChunk{}, err
	}

	return chunk, nil
}

func readColumnMetadata(d *thrift.Decoder) (ColumnMetadata, error) {
	var lastFieldID int64
	var metadata ColumnMetadata
	seen := map[int64]bool{}

	for {
		fieldID, fieldType, err := d.FieldHeader(lastFieldID)
		if err != nil {
			return ColumnMetadata{}, fmt.Errorf("unable to parse field header: %w", err)
		}

		if fieldType == thrift.TypeStop {
			break
		}

		switch fieldID {
		case 1:
			v, err := d.Int64()
			if err != nil {
				return ColumnMetadata{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			metadata.Type = PhysicalType(v)
		case 2:
			count, elemType, err := d.ListHeader()
			if err != nil {
				return ColumnMetadata{}, fmt.Errorf("field %d: %w", fieldID, err)
			}

			if elemType != 5 {
				return ColumnMetadata{}, fmt.Errorf("field %d: encodings list holds type %d, but want i32", fieldID, elemType)
			}

			for range count {
				v, err := d.Int64()
				if err != nil {
					return ColumnMetadata{}, fmt.Errorf("field %d: %w", fieldID, err)
				}
				metadata.Encodings = append(metadata.Encodings, Encoding(v))
			}
		case 3:
			count, elemType, err := d.ListHeader()
			if err != nil {
				return ColumnMetadata{}, fmt.Errorf("field %d: %w", fieldID, err)
			}

			if elemType != 8 {
				return ColumnMetadata{}, fmt.Errorf("field%d: path_in_schema list holds type %d, but want binary", fieldID, elemType)
			}

			for range count {
				v, err := d.Text()
				if err != nil {
					return ColumnMetadata{}, fmt.Errorf("field %d: %w", fieldID, err)
				}
				metadata.PathInSchema = append(metadata.PathInSchema, v)
			}
		case 4:
			v, err := d.Int64()
			if err != nil {
				return ColumnMetadata{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			metadata.Codec = CompressionCodec(v)
		case 5:
			v, err := d.Int64()
			if err != nil {
				return ColumnMetadata{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			metadata.NumValues = v
		case 6:
			v, err := d.Int64()
			if err != nil {
				return ColumnMetadata{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			metadata.TotalUncompressedSize = v
		case 7:
			v, err := d.Int64()
			if err != nil {
				return ColumnMetadata{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			metadata.TotalCompressedSize = v
		case 9:
			v, err := d.Int64()
			if err != nil {
				return ColumnMetadata{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			metadata.DataPageOffset = v
		case 11:
			v, err := d.Int64()
			if err != nil {
				return ColumnMetadata{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			metadata.DictionaryPageOffset = ptr(v)
		case 12:
			stats, err := readStatistics(d)
			if err != nil {
				return ColumnMetadata{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			metadata.Statistics = ptr(stats)
		default:
			err = d.Skip(fieldType)
		}

		if err != nil {
			return ColumnMetadata{}, fmt.Errorf("field %d: %w", fieldID, err)
		}

		seen[fieldID] = true
		lastFieldID = fieldID
	}

	if err := checkRequired(seen, columnMetadataRequired); err != nil {
		return ColumnMetadata{}, err
	}

	return metadata, nil
}

func readStatistics(d *thrift.Decoder) (Statistics, error) {
	var lastFieldID int64
	var stats Statistics

	for {
		fieldID, fieldType, err := d.FieldHeader(lastFieldID)
		if err != nil {
			return Statistics{}, fmt.Errorf("unable to parse field header: %w", err)
		}

		if fieldType == thrift.TypeStop {
			break
		}

		switch fieldID {
		case 3:
			v, err := d.Int64()
			if err != nil {
				return Statistics{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			stats.NullCount = ptr(v)
		case 4:
			v, err := d.Int64()
			if err != nil {
				return Statistics{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			stats.DistinctCount = ptr(v)
		case 5:
			v, err := d.Bytes()
			if err != nil {
				return Statistics{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			stats.MaxValue = v
		case 6:
			v, err := d.Bytes()
			if err != nil {
				return Statistics{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			stats.MinValue = v
		default:
			if err := d.Skip(fieldType); err != nil {
				return Statistics{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
		}

		lastFieldID = fieldID
	}
	return stats, nil
}

func checkRequired(seen map[int64]bool, fields []requiredField) error {
	for _, f := range fields {
		if !seen[f.id] {
			return fmt.Errorf("missing required field %d (%s)", f.id, f.name)
		}
	}
	return nil
}
