package sawdust

import (
	"fmt"

	"github.com/sevaergdm/sawdust/internal/thrift"
)

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
