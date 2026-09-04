package sawdust

import "time"

type ColumnValues interface{ isColumnValues() }

type Int64Values []*int64

func (Int64Values) isColumnValues() {}

type ByteArrayValues []*[]byte

func (ByteArrayValues) isColumnValues() {}

type DoubleValues []*float64

func (DoubleValues) isColumnValues() {}

type BooleanValues []*bool

func (BooleanValues) isColumnValues() {}

type TimestampValues []*time.Time

func (TimestampValues) isColumnValues() {}

type StringValues []*string

func (StringValues) isColumnValues() {}

type ListValues struct {
	Elements ColumnValues // the flat values, as an existing variant
	Offsets  []int        // row i spans Elements[Offsets[i]:Offsets[i+1]]
}

func (ListValues) isColumnValues() {}
