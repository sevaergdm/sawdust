package thrift

import "fmt"

const TypeStop = 0

type Decoder struct {
	buf []byte
	pos int
}

func (d *Decoder) FieldHeader(lastFieldID int64) (int64, byte, error) {
	if d.pos >= len(d.buf) {
		return 0, 0, fmt.Errorf("field header is truncated or malformed")
	}

	b := d.buf[d.pos]
	d.pos++

	if b == 0x00 {
		return 0, 0, nil
	}

	delta := int64(b >> 4)
	fieldType := b & 0x0f

	if fieldType == TypeStop || fieldType > 0x0d {
		return 0, 0, fmt.Errorf("invalid field type, got %d, but must be between 1 and 13", fieldType)
	}

	if delta == 0 {
		fieldID, err := d.Int64()
		if err != nil {
			return 0, 0, err
		}
		return fieldID, fieldType, nil
	}
	return delta + lastFieldID, fieldType, nil
}

func (d *Decoder) Varint() (uint64, error) {
	result := uint64(0)
	shift := uint64(0)
	count := 0

	for {
		if d.pos >= len(d.buf) {
			return 0, fmt.Errorf("varint is truncated")
		}

		b := d.buf[d.pos]
		d.pos++
		count++

		digit := b & 0x7f
		result |= uint64(digit) << shift

		if b < 0x80 {
			return result, nil
		}

		if count >= 10 {
			return 0, fmt.Errorf("varint exceeds max allowed for unit64")
		}

		shift += 7
	}
}

func (d *Decoder) Int64() (int64, error) {
	v, err := d.Varint()
	if err != nil {
		return 0, err
	}

	return fromZigzag(v), nil
}

// Bytes reads a length-prefixed byte string. The returned slice shares memory with the decoder's
// buffer, so callers that keep the value beyond the decode should copy it or use Text() instead
func (d *Decoder) Bytes() ([]byte, error) {
	length, err := d.Varint()
	if err != nil {
		return nil, err
	}

	remaining := uint64(len(d.buf) - d.pos)
	if length > remaining {
		return nil, fmt.Errorf("length (%d) exceeds the buffer size (%d) from position %d", length, len(d.buf), d.pos)
	}

	rawBytes := d.buf[d.pos : d.pos+int(length)]
	d.pos += int(length)

	return rawBytes, nil
}

func (d *Decoder) Text() (string, error) {
	rawBytes, err := d.Bytes()
	if err != nil {
		return "", err
	}
	return string(rawBytes), nil
}

func fromZigzag(n uint64) int64 {
	return int64((n >> 1) ^ -(n & 1))
}

func toZigzag(n int64) uint64 {
	return uint64((n << 1) ^ (n >> 63))
}
