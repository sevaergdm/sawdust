package sawdust

import (
	"encoding/binary"
	"fmt"

	"github.com/sevaergdm/sawdust/internal/thrift"
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
