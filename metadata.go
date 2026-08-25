package sawdust

import (
	"fmt"

	"github.com/sevaergdm/sawdust/internal/thrift"
)

type FileMetadata struct {
	Version   int64           // field id 1, required, version
	Schema    []SchemaElement // field id 2, required, schema
	NumRows   int64           // field id 3, required, num_rows
	RowGroups []RowGroup      // field id 4, required, row_groups
	CreatedBy *string         // field id 6, optional, created_by
}

var fileMetadataRequired = []requiredField{
	{id: 1, name: "version"},
	{id: 2, name: "schema"},
	{id: 3, name: "num_rows"},
	{id: 4, name: "row_groups"},
}

func (m FileMetadata) CreatedByOrEmpty() string {
	if m.CreatedBy == nil {
		return ""
	}
	return *m.CreatedBy
}

func ReadFileMetadata(footer []byte) (FileMetadata, error) {
	var lastFieldID, version, numRows int64
	var fileMetadata FileMetadata

	d := thrift.NewDecoder(footer)
	seen := map[int64]bool{}

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
			fileMetadata.Version = version
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
				fileMetadata.Schema = append(fileMetadata.Schema, elem)
			}
		case 3:
			numRows, err = d.Int64()
			if err != nil {
				return FileMetadata{}, fmt.Errorf("encountered an error parsing num_rows: %w", err)
			}
			fileMetadata.NumRows = numRows
		case 4:
			count, elemType, err := d.ListHeader()
			if err != nil {
				return FileMetadata{}, fmt.Errorf("encountered an error parsing row group elements: %w", err)
			}

			if elemType != 12 {
				return FileMetadata{}, fmt.Errorf("row group list holds type %d, want struct", elemType)
			}

			for range count {
				rowGroup, err := readRowGroup(d)
				if err != nil {
					return FileMetadata{}, fmt.Errorf("encountered an error reading row group: %w", err)
				}
				fileMetadata.RowGroups = append(fileMetadata.RowGroups, rowGroup)
			}
		case 6:
			createdByText, err := d.Text()
			if err != nil {
				return FileMetadata{}, fmt.Errorf("encountered an error parsing created_by: %w", err)
			}
			fileMetadata.CreatedBy = ptr(createdByText)
		default:
			err := d.Skip(fieldType)
			if err != nil {
				return FileMetadata{}, fmt.Errorf("encountered an error skipping type %d: %w", fieldType, err)
			}
		}
		seen[fieldID] = true
		lastFieldID = fieldID
	}

	if err := checkRequired(seen, fileMetadataRequired); err != nil {
		return FileMetadata{}, err
	}

	return fileMetadata, nil
}
