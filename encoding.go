package sawdust

import (
	"encoding/binary"
	"fmt"
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
