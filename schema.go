package sawdust

import (
	"fmt"

	"github.com/sevaergdm/sawdust/internal/thrift"
)

func ptr[T any](v T) *T { return &v }

type LogicalType interface{ isLogicalType() }

type StringType struct{}

func (StringType) isLogicalType() {}

type TimestampType struct {
	IsAdjustedToUTC bool
	Unit            TimeUnit
}

func (TimestampType) isLogicalType() {}

type IntType struct {
	BitWidth int64
	IsSigned bool
}

func (IntType) isLogicalType() {}

type UnknownType struct{ FieldID int64 }

func (UnknownType) isLogicalType() {}

var logicalTypeName = map[int64]string{
	1: "STRING", 2: "MAP", 3: "LIST", 4: "ENUM",
	5: "DECIMAL", 6: "DATE", 7: "TIME", 8: "TIMESTAMP",
	10: "INTEGER", 11: "UNKNOWN", 12: "JSON", 13: "BSON",
	14: "UUID", 15: "FLOAT16", 16: "VARIANT",
	17: "GEOMETRY", 18: "GEOGRAPHY", 19: "FILE",
}

func (u UnknownType) String() string {
	if name, ok := logicalTypeName[u.FieldID]; ok {
		return name + " (not decoded)"
	}
	return fmt.Sprintf("LogicalType(%d) (not decoded)", u.FieldID)
}

func readTimeUnit(d *thrift.Decoder) (TimeUnit, error) {
	var lastFieldID int64
	var result TimeUnit
	for {
		fieldID, fieldType, err := d.FieldHeader(lastFieldID)
		if err != nil {
			return 0, fmt.Errorf("unable to parse field header: %w", err)
		}

		if fieldType == thrift.TypeStop {
			break
		}

		if result != 0 {
			return 0, fmt.Errorf("union has more than one field set")
		}

		switch fieldID {
		case 1:
			result = TimeMillis
		case 2:
			result = TimeMicros
		case 3:
			result = TimeNanos
		default:
			return 0, fmt.Errorf("TimeUnit(%d) is not valid", fieldID)
		}

		if err := d.Skip(fieldType); err != nil {
			return 0, err
		}
		lastFieldID = fieldID
	}

	if result == 0 {
		return 0, fmt.Errorf("TimeUnit is not set")
	}
	return result, nil
}

func readTimestampType(d *thrift.Decoder) (TimestampType, error) {
	var lastFieldID int64
	var result TimestampType
	var sawAdjustedUTC bool
	for {
		fieldID, fieldType, err := d.FieldHeader(lastFieldID)
		if err != nil {
			return TimestampType{}, fmt.Errorf("unable to parse field header: %w", err)
		}

		if fieldType == thrift.TypeStop {
			break
		}

		switch fieldID {
		case 1:
			v, err := d.Bool(fieldType)
			if err != nil {
				return TimestampType{}, fmt.Errorf("field: %d: %w", fieldID, err)
			}
			result.IsAdjustedToUTC = v
			sawAdjustedUTC = true
		case 2:
			timeUnit, err := readTimeUnit(d)
			if err != nil {
				return TimestampType{}, fmt.Errorf("field: %d: %w", fieldID, err)
			}
			result.Unit = timeUnit
		default:
			if err := d.Skip(fieldType); err != nil {
				return TimestampType{}, fmt.Errorf("field: %d: %w", fieldID, err)
			}
		}
		lastFieldID = fieldID
	}

	if !sawAdjustedUTC {
		return TimestampType{}, fmt.Errorf("missing IsAdjustedToUTC in TimestampType")
	}

	if result.Unit == 0 {
		return TimestampType{}, fmt.Errorf("missing Unit in TimestampType")
	}
	return result, nil
}

func readIntType(d *thrift.Decoder) (IntType, error) {
	var lastFieldID int64
	var result IntType
	var sawIsSigned bool
	for {
		fieldID, fieldType, err := d.FieldHeader(lastFieldID)
		if err != nil {
			return IntType{}, err
		}

		if fieldType == thrift.TypeStop {
			break
		}

		switch fieldID {
		case 1:
			v, err := d.Int8()
			if err != nil {
				return IntType{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			result.BitWidth = int64(v)
		case 2:
			v, err := d.Bool(fieldType)
			if err != nil {
				return IntType{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
			result.IsSigned = v
			sawIsSigned = true
		default:
			if err := d.Skip(fieldType); err != nil {
				return IntType{}, fmt.Errorf("field %d: %w", fieldID, err)
			}
		}
		lastFieldID = fieldID
	}

	if !sawIsSigned {
		return IntType{}, fmt.Errorf("missing IsSigned in IntType")
	}

	if result.BitWidth == 0 {
		return IntType{}, fmt.Errorf("missing BitWidth in IntType")
	}
	return result, nil
}

func readLogicalType(d *thrift.Decoder) (LogicalType, error) {
	var lastFieldID int64
	var result LogicalType
	for {
		fieldID, fieldType, err := d.FieldHeader(lastFieldID)
		if err != nil {
			return nil, fmt.Errorf("unable to parse field header: %w", err)
		}

		if fieldType == thrift.TypeStop {
			break
		}

		switch fieldID {
		case 1:
			if err := d.Skip(fieldType); err != nil {
				return nil, err
			}
			result = StringType{}
		case 8:
			timestampType, err := readTimestampType(d)
			if err != nil {
				return nil, err
			}
			result = timestampType
		case 10:
			intType, err := readIntType(d)
			if err != nil {
				return nil, err
			}
			result = intType
		default:
			if err := d.Skip(fieldType); err != nil {
				return nil, err
			}
			result = UnknownType{FieldID: fieldID}
		}
		lastFieldID = fieldID
	}
	return result, nil
}

type SchemaElement struct {
	Type           *PhysicalType        // field id 1, optional, type
	TypeLength     *int64               // field id 2, optional, type_length
	RepetitionType *FieldRepetitionType // field 3, optional, repetition_type
	Name           string               // field 4, required, name
	NumChildren    *int64               // field 5, optional, num_children
	ConvertedType  *ConvertedType       // field 6, optional, converted_type
	LogicalType    LogicalType          // field 10, optional, logical_type
}

func (e SchemaElement) ChildCount() int64 {
	if e.NumChildren == nil {
		return 0
	}
	return *e.NumChildren
}

type SchemaNode struct {
	Element  SchemaElement
	Children []SchemaNode
}

func BuildTree(elements []SchemaElement) (SchemaNode, error) {
	if len(elements) == 0 {
		return SchemaNode{}, fmt.Errorf("schema is empty")
	}

	node, next, err := buildNode(elements, 0)
	if err != nil {
		return SchemaNode{}, err
	}

	if next != len(elements) {
		return SchemaNode{}, fmt.Errorf("schema has %d elements, but the tree consumed %d", len(elements), next)
	}

	return node, nil
}

func buildNode(elements []SchemaElement, i int) (SchemaNode, int, error) {
	var node SchemaNode

	if i >= len(elements) {
		return SchemaNode{}, 0, fmt.Errorf("index exceeds total available elements")
	}

	node.Element = elements[i]
	i++

	for range node.Element.ChildCount() {
		childNode, next, err := buildNode(elements, i)
		if err != nil {
			return SchemaNode{}, 0, err
		}
		node.Children = append(node.Children, childNode)
		i = next
	}
	return node, i, nil
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
		case 6: // converted_type
			v, err := d.Int64()
			if err != nil {
				return SchemaElement{}, err
			}
			elem.ConvertedType = ptr(ConvertedType(v))
		case 10: // logical_type
			logicalType, err := readLogicalType(d)
			if err != nil {
				return SchemaElement{}, err
			}
			elem.LogicalType = logicalType
		default:
			if err := d.Skip(fieldType); err != nil {
				return SchemaElement{}, err
			}
		}
		lastFieldID = fieldID
	}
	return elem, nil
}
