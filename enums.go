package sawdust

import "fmt"

type PhysicalType int
type FieldRepetitionType int
type ConvertedType int
type TimeUnit int

const (
	TypeBoolean           PhysicalType = 0
	TypeInt32             PhysicalType = 1
	TypeInt64             PhysicalType = 2
	TypeInt96             PhysicalType = 3
	TypeFloat             PhysicalType = 4
	TypeDouble            PhysicalType = 5
	TypeByteArray         PhysicalType = 6
	TypeFixedLenByteArray PhysicalType = 7

	RepetitionRequired FieldRepetitionType = 0
	RepetitionOptional FieldRepetitionType = 1
	RepetitionRepeated FieldRepetitionType = 2

	ConvertedUTF8            ConvertedType = 0
	ConvertedMap             ConvertedType = 1
	ConvertedMapKeyValue     ConvertedType = 2
	ConvertedList            ConvertedType = 3
	ConvertedEnum            ConvertedType = 4
	ConvertedDecimal         ConvertedType = 5
	ConvertedDate            ConvertedType = 6
	ConvertedTimeMillis      ConvertedType = 7
	ConvertedTimeMicros      ConvertedType = 8
	ConvertedTimestampMillis ConvertedType = 9
	ConvertedTimestampMicros ConvertedType = 10
	ConvertedUint8           ConvertedType = 11
	ConvertedUint16          ConvertedType = 12
	ConvertedUint32          ConvertedType = 13
	ConvertedUint64          ConvertedType = 14
	ConvertedInt8            ConvertedType = 15
	ConvertedInt16           ConvertedType = 16
	ConvertedInt32           ConvertedType = 17
	ConvertedInt64           ConvertedType = 18
	ConvertedJSON            ConvertedType = 19
	ConvertedBSON            ConvertedType = 20
	ConvertedInterval        ConvertedType = 21

	TimeMillis TimeUnit = 1
	TimeMicros TimeUnit = 2
	TimeNanos  TimeUnit = 3
)

var physicalTypeName = map[PhysicalType]string{
	TypeBoolean:           "boolean",
	TypeInt32:             "int32",
	TypeInt64:             "int64",
	TypeInt96:             "int96",
	TypeFloat:             "float",
	TypeDouble:            "double",
	TypeByteArray:         "byte_array",
	TypeFixedLenByteArray: "fixed_len_byte_array",
}

func (pt PhysicalType) String() string {
	if s, ok := physicalTypeName[pt]; ok {
		return s
	}
	return fmt.Sprintf("PhysicalType(%d)", pt)
}

var fieldRepetitionTypeName = map[FieldRepetitionType]string{
	RepetitionRequired: "required",
	RepetitionOptional: "optional",
	RepetitionRepeated: "repeated",
}

func (f FieldRepetitionType) String() string {
	if s, ok := fieldRepetitionTypeName[f]; ok {
		return s
	}
	return fmt.Sprintf("FieldRepetitionType(%d)", f)
}

var convertedTypeName = map[ConvertedType]string{
	ConvertedUTF8:            "utf8",
	ConvertedMap:             "map",
	ConvertedMapKeyValue:     "map_key_value",
	ConvertedList:            "list",
	ConvertedEnum:            "enum",
	ConvertedDecimal:         "decimal",
	ConvertedDate:            "date",
	ConvertedTimeMillis:      "time_millis",
	ConvertedTimeMicros:      "time_micros",
	ConvertedTimestampMillis: "timestamp_millis",
	ConvertedTimestampMicros: "timestamp_micros",
	ConvertedUint8:           "uint_8",
	ConvertedUint16:          "uint_16",
	ConvertedUint32:          "uint_32",
	ConvertedUint64:          "uint_64",
	ConvertedInt8:            "int_8",
	ConvertedInt16:           "int_16",
	ConvertedInt32:           "int_32",
	ConvertedInt64:           "int_64",
	ConvertedJSON:            "json",
	ConvertedBSON:            "bson",
	ConvertedInterval:        "interval",
}

func (c ConvertedType) String() string {
	if s, ok := convertedTypeName[c]; ok {
		return s
	}
	return fmt.Sprintf("ConvertedType(%d)", c)
}

var timeUnitName = map[TimeUnit]string{
	TimeMillis: "millis",
	TimeMicros: "micros",
	TimeNanos:  "nanos",
}

func (t TimeUnit) String() string {
	if s, ok := timeUnitName[t]; ok {
		return s
	}
	return fmt.Sprintf("TimeUnit(%d)", t)
}
