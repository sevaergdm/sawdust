package sawdust

import (
	"encoding/binary"
	"fmt"
	"math"
)

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
