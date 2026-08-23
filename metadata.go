package sawdust

import (
	"fmt"

	"github.com/sevaergdm/sawdust/internal/thrift"
)

type FileMetadata struct {
	Version   int64           // field id 1, required, version
	Schema    []SchemaElement // field id 2, required, schema
	NumRows   int64           // field id 3, required, num_rows
	CreatedBy *string         // field id 6, optional, created_by
}

func (m FileMetadata) CreatedByOrEmpty() string {
	if m.CreatedBy == nil {
		return ""
	}
	return *m.CreatedBy
}

func ReadFileMetadata(footer []byte) (FileMetadata, error) {
	d := thrift.NewDecoder(footer)
	var lastFieldID, version, numRows int64
	var createdBy *string
	var sawVersion, sawNumRows, sawSchema bool
	var schema []SchemaElement

	for {
		fieldID, fieldType, err := d.FieldHeader(lastFieldID)
		if err != nil {
			return FileMetadata{}, fmt.Errorf("unable to parse field header: %w", err)
		}

		if fieldType == thrift.TypeStop {
			break
		}

		switch fieldID {
		case 1:
			version, err = d.Int64()
			if err != nil {
				return FileMetadata{}, fmt.Errorf("encountered an error parsing version: %w", err)
			}
			sawVersion = true
		case 2:
			count, elemType, err := d.ListHeader()
			if err != nil {
				return FileMetadata{}, fmt.Errorf("encountered an error parsing schema elements: %w", err)
			}

			if elemType != 12 {
				return FileMetadata{}, fmt.Errorf("schema list holds type %d, want struct", elemType)
			}

			for range count {
				elem, err := readSchemaElement(d)
				if err != nil {
					return FileMetadata{}, fmt.Errorf("encountered an error reading schema element: %w", err)
				}
				schema = append(schema, elem)
			}
			sawSchema = true
		case 3:
			numRows, err = d.Int64()
			if err != nil {
				return FileMetadata{}, fmt.Errorf("encountered an error parsing num_rows: %w", err)
			}
			sawNumRows = true
		case 6:
			createdByText, err := d.Text()
			if err != nil {
				return FileMetadata{}, fmt.Errorf("encountered an error parsing created_by: %w", err)
			}
			createdBy = &createdByText
		default:
			err := d.Skip(fieldType)
			if err != nil {
				return FileMetadata{}, fmt.Errorf("encountered an error skipping type %d: %w", fieldType, err)
			}
		}
		lastFieldID = fieldID
	}

	if !sawVersion {
		return FileMetadata{}, fmt.Errorf("footer is missing required field version (1)")
	}

	if !sawNumRows {
		return FileMetadata{}, fmt.Errorf("footer is missing required field num_rows (3)")
	}

	if !sawSchema {
		return FileMetadata{}, fmt.Errorf("footer is missing required field schema (2)")
	}

	return FileMetadata{
		Version:   version,
		Schema:    schema,
		NumRows:   numRows,
		CreatedBy: createdBy,
	}, nil
}
