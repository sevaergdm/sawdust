package sawdust

import (
	"encoding/binary"
	"fmt"
	"io"
)

type Footer struct {
	Length int64
	Start  int64
}

func ReadFooter(f io.ReaderAt, size int64) (Footer, error) {
	if size < 12 {
		return Footer{}, fmt.Errorf("malformed file, expected at least 12 bytes, but got %d", size)
	}

	head := make([]byte, 4)
	tail := make([]byte, 8)

	if _, err := f.ReadAt(head, 0); err != nil {
		return Footer{}, fmt.Errorf("unable to read first 4 bytes: %w", err)
	}

	if _, err := f.ReadAt(tail, size-8); err != nil {
		return Footer{}, fmt.Errorf("unable to read tail: %w", err)
	}

	if string(head) != "PAR1" || string(tail[4:8]) != "PAR1" {
		return Footer{}, fmt.Errorf("expected 'PAR1' as head and tail of file, but got: %q and %q", head, tail[4:8])
	}

	footerLength := int64(binary.LittleEndian.Uint32(tail[:4]))
	metadataStart := size - 8 - footerLength
	if metadataStart < 4 {
		return Footer{}, fmt.Errorf("metadata start position should be at least 4, got: %d", metadataStart)
	}

	return Footer{Length: footerLength, Start: metadataStart}, nil
}
