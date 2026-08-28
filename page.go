package sawdust

import (
	"fmt"

	"github.com/klauspost/compress/zstd"
	"github.com/sevaergdm/sawdust/internal/thrift"
)

type Page struct {
	Header PageHeader
	Data   []byte
}

func (p Page) decompressZstd(d *zstd.Decoder) ([]byte, error) {
	dataPageHeader := p.Header.DataPageHeaderV2
	if dataPageHeader == nil {
		return nil, fmt.Errorf("no header data available for decoding")
	}

	levelsLen := dataPageHeader.RepetitionLevelsByteLength + dataPageHeader.DefinitionLevelsByteLength
	if levelsLen < 0 || int(levelsLen) > len(p.Data) {
		return nil, fmt.Errorf("malformed level length (%d)", levelsLen)
	}

	wantLen := int(p.Header.UncompressedPageSize - levelsLen)
	if wantLen < 0 {
		return nil, fmt.Errorf("uncompressed page size (%d) exceeds total levels length (%d)", p.Header.UncompressedPageSize, levelsLen)
	}

	if !dataPageHeader.IsCompressed {
		if len(p.Data[levelsLen:]) != wantLen {
			return nil, fmt.Errorf("expected %d bytes, but got %d", wantLen, len(p.Data[levelsLen:]))
		}
		return p.Data[levelsLen:], nil
	}

	out := make([]byte, 0, wantLen)
	decodedBytes, err := d.DecodeAll(p.Data[levelsLen:], out)
	if err != nil {
		return nil, fmt.Errorf("unexpected error decoding payload: %w", err)
	}

	if len(decodedBytes) != wantLen {
		return nil, fmt.Errorf("expected %d bytes after decoding, but got %d", wantLen, len(decodedBytes))
	}
	return decodedBytes, nil
}

type PageHeader struct {
	Type                 PageType          // field 1, required
	UncompressedPageSize int64             // field 2, required
	CompressedPageSize   int64             // field 3, required
	CRC                  *int64            // field 4, optional
	DataPageHeaderV2     *DataPageHeaderV2 // field 8, optional
}

func (p PageHeader) ValueCount() int64 {
	if p.DataPageHeaderV2 == nil {
		return 0
	}
	return p.DataPageHeaderV2.NumValues
}

var pageHeaderRequired = []requiredField{
	{id: 1, name: "type"},
	{id: 2, name: "uncompressed_page_size"},
	{id: 3, name: "compressed_page_size"},
}

type DataPageHeaderV2 struct {
	NumValues                  int64       // field 1, required
	NumNulls                   int64       // field 2, required
	NumRows                    int64       // field 3, required
	Encoding                   Encoding    // field 4, required
	DefinitionLevelsByteLength int64       // field 5, required
	RepetitionLevelsByteLength int64       // field 6, required
	IsCompressed               bool        // field 7, optional, default is true
	Statistics                 *Statistics // field 8, optional
}

var dataPageHeaderV2Required = []requiredField{
	{id: 1, name: "num_values"},
	{id: 2, name: "num_nulls"},
	{id: 3, name: "num_rows"},
	{id: 4, name: "encoding"},
	{id: 5, name: "definition_levels_byte_length"},
	{id: 6, name: "repetition_levels_byte_length"},
}

func readPageHeader(d *thrift.Decoder) (PageHeader, error) {
	var lastFieldID int64
	var pageHeader PageHeader
	seen := map[int64]bool{}
	for {
		fieldID, fieldType, err := d.FieldHeader(lastFieldID)
		if err != nil {
			return PageHeader{}, fmt.Errorf("unable to parse field header: %w", err)
		}

		if fieldType == thrift.TypeStop {
			break
		}

		switch fieldID {
		case 1:
			v, err := d.Int64()
			if err != nil {
				return PageHeader{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			pageHeader.Type = PageType(v)
		case 2:
			v, err := d.Int64()
			if err != nil {
				return PageHeader{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			pageHeader.UncompressedPageSize = v
		case 3:
			v, err := d.Int64()
			if err != nil {
				return PageHeader{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			pageHeader.CompressedPageSize = v
		case 4:
			v, err := d.Int64()
			if err != nil {
				return PageHeader{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			pageHeader.CRC = ptr(v)
		case 8:
			v, err := readDataPageHeaderV2(d)
			if err != nil {
				return PageHeader{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			pageHeader.DataPageHeaderV2 = ptr(v)
		default:
			if err := d.Skip(fieldType); err != nil {
				return PageHeader{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
		}
		seen[fieldID] = true
		lastFieldID = fieldID
	}

	if err := checkRequired(seen, pageHeaderRequired); err != nil {
		return PageHeader{}, err
	}

	if pageHeader.Type == DataPageV2 && !seen[8] {
		return PageHeader{}, fmt.Errorf("malformed input, missing DataPageHeaderV2")
	}

	if pageHeader.Type != DataPageV2 {
		return PageHeader{}, fmt.Errorf("page header type %d (%s) is not supported", pageHeader.Type, pageHeader.Type)
	}

	return pageHeader, nil
}

func readDataPageHeaderV2(d *thrift.Decoder) (DataPageHeaderV2, error) {
	var lastFieldID int64
	var dataPageHeaderV2 DataPageHeaderV2
	dataPageHeaderV2.IsCompressed = true
	seen := map[int64]bool{}

	for {
		fieldID, fieldType, err := d.FieldHeader(lastFieldID)
		if err != nil {
			return DataPageHeaderV2{}, fmt.Errorf("unable to parse field header: %w", err)
		}

		if fieldType == thrift.TypeStop {
			break
		}

		switch fieldID {
		case 1:
			v, err := d.Int64()
			if err != nil {
				return DataPageHeaderV2{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			dataPageHeaderV2.NumValues = v
		case 2:
			v, err := d.Int64()
			if err != nil {
				return DataPageHeaderV2{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			dataPageHeaderV2.NumNulls = v
		case 3:
			v, err := d.Int64()
			if err != nil {
				return DataPageHeaderV2{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			dataPageHeaderV2.NumRows = v
		case 4:
			v, err := d.Int64()
			if err != nil {
				return DataPageHeaderV2{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			dataPageHeaderV2.Encoding = Encoding(v)
		case 5:
			v, err := d.Int64()
			if err != nil {
				return DataPageHeaderV2{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			dataPageHeaderV2.DefinitionLevelsByteLength = v
		case 6:
			v, err := d.Int64()
			if err != nil {
				return DataPageHeaderV2{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			dataPageHeaderV2.RepetitionLevelsByteLength = v
		case 7:
			v, err := d.Bool(fieldType)
			if err != nil {
				return DataPageHeaderV2{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			dataPageHeaderV2.IsCompressed = v
		case 8:
			v, err := readStatistics(d)
			if err != nil {
				return DataPageHeaderV2{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			dataPageHeaderV2.Statistics = ptr(v)
		default:
			if err := d.Skip(fieldType); err != nil {
				return DataPageHeaderV2{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
		}
		seen[fieldID] = true
		lastFieldID = fieldID
	}

	if err := checkRequired(seen, dataPageHeaderV2Required); err != nil {
		return DataPageHeaderV2{}, err
	}

	return dataPageHeaderV2, nil
}
