package sawdust

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

type Footer struct {
	Length int64
	Start  int64
}

type File struct {
	Size     int64
	Footer   Footer
	Metadata FileMetadata
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

func ReadFile(path string) (File, error) {
	f, err := os.Open(path)
	if err != nil {
		return File{}, fmt.Errorf("encountered an error opening file %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	fStat, err := f.Stat()
	if err != nil {
		return File{}, fmt.Errorf("could not stat %q: %w", path, err)
	}

	size := fStat.Size()

	footer, err := ReadFooter(f, size)
	if err != nil {
		return File{}, fmt.Errorf("encountered an error reading footer in %q: %v", path, err)
	}

	footerBytes := make([]byte, footer.Length)
	if _, err := f.ReadAt(footerBytes, footer.Start); err != nil {
		return File{}, fmt.Errorf("encountered an error reading from offset in %q: %v", path, err)
	}

	fileMetadata, err := ReadFileMetadata(footerBytes)
	if err != nil {
		return File{}, fmt.Errorf("encountered an error fetching file metadata in %q: %v", path, err)
	}
	return File{Size: size, Footer: footer, Metadata: fileMetadata}, nil
}
