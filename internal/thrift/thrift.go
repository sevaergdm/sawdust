package thrift

import (
	"fmt"
	"math"
)

const TypeStop = 0

type Decoder struct {
	buf []byte
	pos int
}

func NewDecoder(buf []byte) *Decoder {
	return &Decoder{buf: buf}
}

func (d *Decoder) Pos() int {
	return d.pos
}

func (d *Decoder) Skip(typ byte) error {
	switch typ {
	case 0x01, 0x02:
		return nil
	case 0x03:
		if len(d.buf)-d.pos < 1 {
			return fmt.Errorf("insufficient space left in buffer")
		}
		d.pos++
		return nil
	case 0x04, 0x05, 0x06:
		_, err := d.Varint()
		return err
	case 0x07:
		if len(d.buf)-d.pos < 8 {
			return fmt.Errorf("insufficient space left in buffer")
		}
		d.pos += 8
		return nil
	case 0x08:
		_, err := d.Bytes()
		return err
	case 0x09, 0x0a:
		size, elemType, err := d.ListHeader()
		if err != nil {
			return err
		}

		for range size {
			err := d.Skip(elemType)
			if err != nil {
				return err
			}
		}
	case 0x0b:
		size, keyType, valueType, err := d.MapHeader()
		if err != nil {
			return err
		}

		for range size {
			err := d.Skip(keyType)
			if err != nil {
				return err
			}

			err = d.Skip(valueType)
			if err != nil {
				return err
			}
		}
	case 0x0c:
		var lastFieldID int64
		for {
			fieldID, fieldType, err := d.FieldHeader(lastFieldID)
			if err != nil {
				return err
			}

			if fieldID == 0 && fieldType == 0 {
				break
			}

			if err := d.Skip(fieldType); err != nil {
				return err
			}
			lastFieldID = fieldID
		}
	case 0x0d:
		if len(d.buf)-d.pos < 16 {
			return fmt.Errorf("insufficient space left in buffer")
		}
		d.pos += 16
	default:
		return fmt.Errorf("%d is an invalid type", typ)
	}
	return nil
}

func (d *Decoder) MapHeader() (size int, keyType, valueType byte, err error) {
	s, err := d.Varint()
	if err != nil {
		return 0, 0, 0, err
	}

	if s == 0 {
		return 0, 0, 0, nil
	}

	if s > math.MaxInt32 {
		return 0, 0, 0, fmt.Errorf("size (%d) exceeds maximum possible value (%d)", s, math.MaxInt32)
	}

	if d.pos >= len(d.buf) {
		return 0, 0, 0, fmt.Errorf("map header is truncated or malformed")
	}

	b := d.buf[d.pos]
	d.pos++

	keyType = b >> 4
	valueType = b & 0x0f

	if keyType < 1 || keyType > 13 {
		return 0, 0, 0, fmt.Errorf("key type should be in range 1 to 13, but got %d", keyType)
	}

	if valueType < 1 || valueType > 13 {
		return 0, 0, 0, fmt.Errorf("value type should be in range 1 to 13, but got %d", valueType)
	}
	return int(s), keyType, valueType, nil
}

func (d *Decoder) ListHeader() (int, byte, error) {
	if d.pos >= len(d.buf) {
		return 0, 0, fmt.Errorf("list header is truncated or malformed")
	}

	b := d.buf[d.pos]
	d.pos++

	size := b >> 4
	elemType := b & 0x0f

	if elemType < 0x01 || elemType > 0x0d {
		return 0, 0, fmt.Errorf("invalid element type, got %d, but must be between 1 and 13", elemType)
	}

	if size == 0x0f {
		longSize, err := d.Varint()
		if err != nil {
			return 0, 0, err
		}

		if longSize < 15 {
			return 0, 0, fmt.Errorf("long form expects size to exceed 14, but got %d", longSize)
		}

		if longSize > math.MaxInt32 {
			return 0, 0, fmt.Errorf("size (%d) exceeds maximum possible value (%d)", longSize, math.MaxInt32)
		}

		return int(longSize), elemType, nil
	}

	return int(size), elemType, nil
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

func (d *Decoder) Int8() (int8, error) {
	if d.pos >= len(d.buf) {
		return 0, fmt.Errorf("int8 is truncated")
	}
	b := d.buf[d.pos]
	d.pos++
	return int8(b), nil
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

func (d *Decoder) Bool(fieldType byte) (bool, error) {
	switch fieldType {
	case 0x01:
		return true, nil
	case 0x02:
		return false, nil
	default:
		return false, fmt.Errorf("boolean field type must be 1 or 2, but got %d", fieldType)
	}
}

func fromZigzag(n uint64) int64 {
	return int64((n >> 1) ^ -(n & 1))
}

func toZigzag(n int64) uint64 {
	return uint64((n << 1) ^ (n >> 63))
}
