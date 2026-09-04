package sawdust

import (
	"fmt"

	"github.com/klauspost/compress/zstd"
)

func (f *File) decompress(p Page, codec CompressionCodec) ([]byte, error) {
	var levelsLen int64
	var compressed bool

	if p.Header.Type == DictionaryPage {
		levelsLen = 0
		compressed = codec != CodecUncompressed
	} else {
		h := p.Header.DataPageHeaderV2
		if h == nil {
			return nil, fmt.Errorf("no data page header available")
		}
		levelsLen = h.RepetitionLevelsByteLength + h.DefinitionLevelsByteLength
		compressed = h.IsCompressed
	}

	if levelsLen < 0 || int(levelsLen) > len(p.Data) {
		return nil, fmt.Errorf("malformed level length (%d)", levelsLen)
	}

	wantLen := int(p.Header.UncompressedPageSize - levelsLen)
	if wantLen < 0 {
		return nil, fmt.Errorf("uncompressed page size (%d) exceeds total levels length (%d)", p.Header.UncompressedPageSize, levelsLen)
	}
	payload := p.Data[levelsLen:]

	var out []byte
	if !compressed {
		out = payload
	} else {
		switch codec {
		case CodecZSTD:
			dec, err := f.zstdDecoder()
			if err != nil {
				return nil, err
			}
			out, err = dec.DecodeAll(payload, make([]byte, 0, wantLen))
			if err != nil {
				return nil, fmt.Errorf("unexpected error decoding payload: %w", err)
			}
		default:
			return nil, fmt.Errorf("codec %s is not supported", codec)
		}
	}

	if len(out) != wantLen {
		return nil, fmt.Errorf("expected %d bytes after decoding, but got %d", wantLen, len(out))
	}
	return out, nil
}

func (f *File) zstdDecoder() (*zstd.Decoder, error) {
	if f.zstdDec == nil {
		dec, err := zstd.NewReader(nil)
		if err != nil {
			return nil, err
		}
		f.zstdDec = dec
	}
	return f.zstdDec, nil
}
