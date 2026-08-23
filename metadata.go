package sawdust

import (
	"fmt"

	"github.com/sevaergdm/sawdust/internal/thrift"
)

type FileMetadata struct {
	Version   int64
	NumRows   int64
	CreatedBy *string
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
	var sawVersion, sawNumRows bool

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

	return FileMetadata{Version: version, NumRows: numRows, CreatedBy: createdBy}, nil
}
