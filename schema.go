package sawdust

import (
	"fmt"
	"slices"

	"github.com/sevaergdm/sawdust/internal/thrift"
)

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

type SchemaColumn struct {
	Path               []string
	Element            SchemaElement
	MaxDefinitionLevel int
	MaxRepetitionLevel int
}

func Columns(root SchemaNode) []SchemaColumn {
	var columns []SchemaColumn
	for _, child := range root.Children {
		columns = collectColumns(child, nil, 0, 0, columns)
	}
	return columns
}

func collectColumns(node SchemaNode, path []string, def, rep int, out []SchemaColumn) []SchemaColumn {
	if rt := node.Element.RepetitionType; rt != nil {
		if *rt == RepetitionOptional || *rt == RepetitionRepeated {
			def++
		}

		if *rt == RepetitionRepeated {
			rep++
		}
	}

	path = append(path, node.Element.Name)

	if len(node.Children) == 0 {
		out = append(out, SchemaColumn{Path: slices.Clone(path), Element: node.Element, MaxDefinitionLevel: def, MaxRepetitionLevel: rep})
		return out
	}

	for _, child := range node.Children {
		out = collectColumns(child, path, def, rep, out)
	}
	return out
}
