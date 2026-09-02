package sawdust

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"

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

type TimestampValues []*time.Time

func (TimestampValues) isColumnValues() {}

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
func decodeRLE(b []byte, bitWidth, count int) ([]int64, error) {
	pos := 0
	out := make([]int64, 0, count)

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
			packedOut, bytesConsumed, err := unpackBits(b[pos:], bitWidth, groups*8)
			if err != nil {
				return nil, err
			}
			out = append(out, packedOut...)
			pos += bytesConsumed
			// RLE
		} else {
			mask := uint64(1)<<bitWidth - 1
			repeats := v >> 1
			numBytes := (bitWidth + 7) / 8
			if numBytes < 0 || pos+numBytes > len(b) {
				return nil, fmt.Errorf("run needs %d bytes at %d, but only %d remain", numBytes, pos, len(b)-pos)
			}
			var val int64
			for i := range numBytes {
				val |= int64(b[pos+i]) << (8 * i)
			}

			if val > int64(mask) {
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
func applyDefinitionLevels[T any](levels []int64, values []T, maxDefLevel int64) ([]*T, error) {
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

func decodeDeltaLengthByteArray(b []byte) ([][]byte, error) {
	lengths, consumed, err := decodeDeltaBinary(b)
	if err != nil {
		return nil, err
	}
	lengthSum := 0
	for _, l := range lengths {
		lengthSum += int(l)
	}
	if lengthSum != len(b)-consumed {
		return nil, fmt.Errorf("mismatch sum of lengths (%d) should match available bytes (%d)", lengthSum, len(b)-consumed)
	}

	out := make([][]byte, 0, len(lengths))
	cursor := consumed
	for _, l := range lengths {
		if l < 0 {
			return nil, fmt.Errorf("malformed individual length: %d should be greater than 0", l)
		}
		out = append(out, b[cursor:cursor+int(l)])
		cursor += int(l)
	}
	return out, nil
}

func decodeDeltaBinary(b []byte) ([]int64, int, error) {
	d := thrift.NewDecoder(b)

	blockSize, err := d.Varint()
	if err != nil {
		return nil, 0, fmt.Errorf("blockSize: %w", err)
	}
	if blockSize%128 != 0 {
		return nil, 0, fmt.Errorf("invalid blocksize, must be a multiple of 128, but got %d", blockSize)
	}

	numMiniblocks, err := d.Varint()
	if err != nil {
		return nil, 0, fmt.Errorf("numMiniblocks: %v", err)
	}
	if numMiniblocks == 0 {
		return nil, 0, fmt.Errorf("numMiniblocks cannot be 0")
	}
	if blockSize%numMiniblocks != 0 {
		return nil, 0, fmt.Errorf("invalid number of miniblocks (%d), must be evenly divisible from block size (%d)", numMiniblocks, blockSize)
	}

	valuesPerMiniblock := blockSize / numMiniblocks
	if valuesPerMiniblock%32 != 0 {
		return nil, 0, fmt.Errorf("invalid number of values per miniblock %d, must be a multiple of 32", valuesPerMiniblock)
	}

	totalValueCount, err := d.Varint()
	if err != nil {
		return nil, 0, fmt.Errorf("totalValueCount: %w", err)
	}

	firstValue, err := d.Int64()
	if err != nil {
		return nil, 0, fmt.Errorf("firstValue: %w", err)
	}

	pos := d.Pos()
	values := []int64{firstValue}

	for len(values) < int(totalValueCount) {
		md := thrift.NewDecoder(b[pos:])
		minDelta, err := md.Int64()
		if err != nil {
			return nil, 0, fmt.Errorf("minDelta: %w", err)
		}
		pos += md.Pos()

		if pos+int(numMiniblocks) > len(b) {
			return nil, 0, fmt.Errorf("number of miniblocks (%d) exceeds total available bytes (%d)", numMiniblocks, len(b))
		}

		bitWidths := b[pos : pos+int(numMiniblocks)]
		pos += int(numMiniblocks)

		for _, w := range bitWidths {
			vals, n, err := unpackBits(b[pos:], int(w), int(valuesPerMiniblock))
			if err != nil {
				return nil, 0, err
			}
			pos += n
			for _, u := range vals {
				prev := values[len(values)-1]
				values = append(values, prev+minDelta+int64(u))
			}
		}
	}
	return values[:totalValueCount], pos, nil
}

func unpackBits(b []byte, bitWidth, count int) ([]int64, int, error) {
	mask := uint64(1)<<bitWidth - 1
	var out []int64
	numBytes := count * bitWidth / 8
	if numBytes < 0 || numBytes > len(b) {
		return nil, 0, fmt.Errorf("run needs %d bytes, but only %d remain", numBytes, len(b))
	}
	for i := range count {
		bitOffset := i * bitWidth
		byteIndex := bitOffset / 8
		shift := bitOffset % 8

		var chunk uint64
		for j := 0; j < 8 && byteIndex+j < len(b); j++ {
			chunk |= uint64(b[byteIndex+j]) << (8 * j)
		}

		val := (chunk >> shift) & mask
		out = append(out, int64(val))
	}

	return out, numBytes, nil
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
