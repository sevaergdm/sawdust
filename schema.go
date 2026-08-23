package sawdust

import (
	"github.com/sevaergdm/sawdust/internal/thrift"
)

func ptr[T any](v T) *T { return &v }

type LogicalType struct {
	String    *StringType    // union field 1
	Timestamp *TimestampType // union field 8
}

type StringType struct{}

type TimestampType struct {
	IsAdjustedToUTC bool
	Unit            TimeUnit
}

type SchemaElement struct {
	Type           *PhysicalType        // field id 1, optional, type
	TypeLength     *int64               // field id 2, optional, type_length
	RepetitionType *FieldRepetitionType // field 3, optional, repetition_type
	Name           string               // field 4, required, name
	NumChildren    *int64               // field 5, optional, num_children
	ConvertedType  *ConvertedType       // field 6, optional, converted_type
	LogicalType    *LogicalType         // field 10, optional, logical_type
}

func readSchemaElement(d *thrift.Decoder) (SchemaElement, error) {
	var elem SchemaElement
	var lastFieldID int64

	for {
		fieldID, fieldType, err := d.FieldHeader(lastFieldID)
		if err != nil {
			return SchemaElement{}, err
		}

		if fieldType == thrift.TypeStop {
			break
		}

		switch fieldID {
		case 1: // type
			v, err := d.Int64()
			if err != nil {
				return SchemaElement{}, err
			}
			elem.Type = ptr(PhysicalType(v))
		case 2: // type_length
			v, err := d.Int64()
			if err != nil {
				return SchemaElement{}, err
			}
			elem.TypeLength = ptr(v)
		case 3: // repetition_type
			v, err := d.Int64()
			if err != nil {
				return SchemaElement{}, err
			}
			elem.RepetitionType = ptr(FieldRepetitionType(v))
		case 4: // name
			elem.Name, err = d.Text()
			if err != nil {
				return SchemaElement{}, err
			}
		case 5: // num_children
			v, err := d.Int64()
			if err != nil {
				return SchemaElement{}, err
			}
			elem.NumChildren = ptr(v)
		default:
			if err := d.Skip(fieldType); err != nil {
				return SchemaElement{}, err
			}
		}
		lastFieldID = fieldID
	}
	return elem, nil
}
