package sawdust

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/sevaergdm/sawdust/internal/thrift"
)

type ColumnValues interface{ isColumnValues() }

type Int64Values []*int64

func (Int64Values) isColumnValues() {}

type ByteArrayValues []*[]byte

func (ByteArrayValues) isColumnValues() {}

type DoubleValues []*float64

func (DoubleValues) isColumnValues() {}

type BooleanValues []*bool

func (BooleanValues) isColumnValues() {}

func decodePlainInt64(b []byte) ([]int64, error) {
	if len(b)%8 != 0 {
		return nil, fmt.Errorf("expected total bytes to be a multiple of 8, but got %d", len(b))
	}

	nums := make([]int64, 0, len(b)/8)
	for i := 0; i < len(b); i += 8 {
		n := binary.LittleEndian.Uint64(b[i:])
		nums = append(nums, int64(n))
	}

	return nums, nil
}

func decodePlainDouble(b []byte) ([]float64, error) {
	if len(b)%8 != 0 {
		return nil, fmt.Errorf("expected total bytes to be a multiple of 8, but got %d", len(b))
	}

	nums := make([]float64, 0, len(b)/8)
	for i := 0; i < len(b); i += 8 {
		n := binary.LittleEndian.Uint64(b[i:])
		nums = append(nums, math.Float64frombits(n))
	}

	return nums, nil
}

func decodePlainBoolean(b []byte, count int) ([]bool, error) {
	if count < 0 {
		return nil, fmt.Errorf("count must not be negative, got %d", count)
	}

	need := (count + 7) / 8
	if need > len(b) {
		return nil, fmt.Errorf("count %d requires %d bytes, but only %d available", count, need, len(b))
	}
	out := make([]bool, 0, count)

	for i := range count {
		v := (b[i/8] >> (i % 8)) & 1
		out = append(out, v == 1)
	}
	return out, nil
}

// decodePlainByteArray decodes PLAIN BYTE_ARRAY values: each is a 4-byte
// little-endian length followed by that many bytes, repeating until the buffer
// is exhausted.
// The returned slices alias b rather than copying it. Mutating b changes the
// returned values, and retaining any one of them keeps b's whole backing array
// alive.
func decodePlainByteArray(b []byte) ([][]byte, error) {
	var out [][]byte

	cursor := 0
	for cursor < len(b) {
		if len(b)-cursor < 4 {
			return nil, fmt.Errorf("malformed values, expected at least 4 bytes, but got %d", len(b)-cursor)
		}
		length := int(binary.LittleEndian.Uint32(b[cursor:]))
		if length > len(b[cursor+4:]) {
			return nil, fmt.Errorf("declared length (%d) exceeds remaining buffer (%d)", length, len(b[cursor+4:]))
		}
		value := b[cursor+4 : cursor+4+length]
		cursor += 4 + length
		out = append(out, value)
	}
	return out, nil
}

// decodeRLE decodes the RLE/bit-packing hybrid (Parquet encoding 3) used for
// both definition levels and dictionary indices. bitWidth is derived from the
// schema for levels, and read from the data for dictionary indices.
func decodeRLE(b []byte, bitWidth, count int) ([]int32, error) {
	pos := 0
	out := make([]int32, 0, count)
	mask := uint64(1)<<bitWidth - 1

	for len(out) < count {
		d := thrift.NewDecoder(b[pos:])
		v, err := d.Varint()
		if err != nil {
			return nil, err
		}
		pos += d.Pos()

		// bit packed
		if v&0x01 == 0x01 {
			groups := int(v >> 1)
			numBytes := groups * bitWidth
			if numBytes < 0 || pos+numBytes > len(b) {
				return nil, fmt.Errorf("run needs %d bytes at %d, but only %d remain", numBytes, pos, len(b)-pos)
			}
			for i := 0; i < groups*8; i++ {
				bitOffset := i * bitWidth
				byteIndex := pos + bitOffset/8
				shift := bitOffset % 8

				var chunk uint64
				for j := 0; j < 8 && byteIndex+j < len(b); j++ {
					chunk |= uint64(b[byteIndex+j]) << (8 * j)
				}

				val := (chunk >> shift) & mask
				out = append(out, int32(val))
			}
			pos += numBytes
			// RLE
		} else {
			repeats := v >> 1
			numBytes := (bitWidth + 7) / 8
			if numBytes < 0 || pos+numBytes > len(b) {
				return nil, fmt.Errorf("run needs %d bytes at %d, but only %d remain", numBytes, pos, len(b)-pos)
			}
			var val int32
			for i := range numBytes {
				val |= int32(b[pos+i]) << (8 * i)
			}

			if val > int32(mask) {
				return nil, fmt.Errorf("value %d exceeds largest value possible in %d bits", val, bitWidth)
			}

			remaining := uint64(count - len(out))
			n := min(repeats, remaining)
			for range n {
				out = append(out, val)
			}
			pos += numBytes
		}
	}
	return out[:count], nil
}

// applyDefinitionLevels expands values into one slot per row. The result has one entry per level:
// a pointer to the next unused value where the level equals maxDefLevel, and nil where it does not,
// since a null consumes no value.
//
// The returned pointers alias values rather than copying it. Mutating values after the call changes
// the result, and retaining any non-nil pointer keeps the whole values backing array alive.
func applyDefinitionLevels[T any](levels []int32, values []T, maxDefLevel int32) ([]*T, error) {
	var out []*T
	cursor := 0

	for _, level := range levels {
		if level == maxDefLevel {
			if cursor >= len(values) {
				return nil, fmt.Errorf("total number of levels (%d) exceeds available values (%d)", len(levels), len(values))
			}
			out = append(out, &values[cursor])
			cursor++
		} else {
			out = append(out, nil)
		}
	}

	if cursor != len(values) {
		return nil, fmt.Errorf("levels said %d values were present, but only %d were stored", len(values), cursor)
	}
	return out, nil
}
