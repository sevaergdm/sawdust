package sawdust

import (
	"fmt"
	"time"

	"github.com/sevaergdm/sawdust/internal/thrift"
)

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

func toTimes(ts []*int64, tsType TimestampType) (TimestampValues, error) {
	out := make([]*time.Time, 0, len(ts))
	for _, t := range ts {
		if t == nil {
			out = append(out, nil)
			continue
		}
		switch tsType.Unit {
		case TimeMillis:
			v := time.UnixMilli(*t).UTC()
			out = append(out, &v)
		case TimeMicros:
			v := time.UnixMicro(*t).UTC()
			out = append(out, &v)
		case TimeNanos:
			v := time.Unix(0, *t).UTC()
			out = append(out, &v)
		default:
			return nil, fmt.Errorf("unsupported time unit %s", tsType.Unit)
		}
	}
	return TimestampValues(out), nil
}

func toStrings(vals []*[]byte) (StringValues, error) {
	out := make([]*string, 0, len(vals))
	for _, v := range vals {
		if v == nil {
			out = append(out, nil)
			continue
		}
		s := string(*v)
		out = append(out, &s)
	}
	return out, nil
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
