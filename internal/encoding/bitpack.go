package encoding

import (
	"fmt"
)

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
