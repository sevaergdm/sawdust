package encoding

import (
	"fmt"

	"github.com/sevaergdm/sawdust/internal/thrift"
)

func DecodeDeltaLengthByteArray(b []byte) ([][]byte, error) {
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
