package thrift

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func TestVarint(t *testing.T) {
	tests := []struct {
		name    string
		buf     []byte
		want    uint64
		wantPos int
		wantErr bool
	}{
		{name: "one byte", buf: []byte{0x01}, want: 1, wantPos: 1},
		{name: "largest one byte", buf: []byte{0x7f}, want: 127, wantPos: 1},
		{name: "smallest two bytes", buf: []byte{0x80, 0x01}, want: 128, wantPos: 2},
		{name: "three hundred", buf: []byte{0xac, 0x02}, want: 300, wantPos: 2},
		{name: "spec example", buf: []byte{0xdf, 0x89, 0x03}, want: 50399, wantPos: 3},

		{name: "empty buffer", buf: []byte{}, wantErr: true},
		{name: "ends mid varint", buf: []byte{0x80}, wantErr: true},
		{name: "too long for 64 bits", buf: []byte{
			0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80,
		}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Decoder{buf: tt.buf}
			got, err := d.Varint()
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}

			if tt.wantErr {
				return
			}

			if got != tt.want {
				t.Errorf("value: want %d, got %d", tt.want, got)
			}

			if d.pos != tt.wantPos {
				t.Errorf("pos: want %d, got %d", tt.wantPos, d.pos)
			}
		})
	}
}

func TestFromZigzag(t *testing.T) {
	tests := []struct {
		name  string
		input uint64
		want  int64
	}{
		{
			name:  "0",
			input: uint64(0),
			want:  int64(0),
		},
		{
			name:  "1",
			input: uint64(1),
			want:  int64(-1),
		},
		{
			name:  "2",
			input: uint64(2),
			want:  int64(1),
		},
		{
			name:  "4",
			input: uint64(4),
			want:  int64(2),
		},
		{
			name:  "16",
			input: uint64(16),
			want:  int64(8),
		},
		{
			name:  "1<<63",
			input: uint64(1 << 63),
			want:  int64(4611686018427387904),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fromZigzag(tt.input)

			if got != tt.want {
				t.Errorf("expected %d, but got %d", tt.want, got)
			}
		})
	}

	t.Run("roundtrip", func(t *testing.T) {
		for v := int64(-100_000); v <= 100_000; v++ {
			if got := fromZigzag(toZigzag(v)); got != v {
				t.Errorf("%d round tripped to %d", v, got)
			}
		}

		for _, v := range []int64{math.MinInt64, math.MaxInt64} {
			if got := fromZigzag(toZigzag(v)); got != v {
				t.Errorf("%d round tripped to %d", v, got)
			}
		}
	})
}

func TestInt64(t *testing.T) {
	tests := []struct {
		name    string
		buf     []byte
		want    int64
		wantPos int
		wantErr bool
	}{
		{name: "one byte: 04", buf: []byte{0x04}, want: 2, wantPos: 1},
		{name: "one byte: 03", buf: []byte{0x03}, want: -2, wantPos: 1},
		{name: "one byte: 10", buf: []byte{0x10}, want: 8, wantPos: 1},
		{name: "two bytes: c0 0c", buf: []byte{0xc0, 0x0c}, want: 800, wantPos: 2},
		{name: "one byte error: 80", buf: []byte{0x80}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Decoder{buf: tt.buf}
			got, err := d.Int64()
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}

			if tt.wantErr {
				if !strings.Contains(err.Error(), "varint") {
					t.Errorf("error message '%v' should contain 'varint'", err)
				}
				return
			}

			if got != tt.want {
				t.Errorf("value: want %d, got %d", tt.want, got)
			}

			if d.pos != tt.wantPos {
				t.Errorf("pos: want %d, got %d", tt.wantPos, d.pos)
			}
		})
	}
}

func TestBytes(t *testing.T) {
	tests := []struct {
		name    string
		buf     []byte
		want    []byte
		wantPos int
		wantErr bool
	}{
		{name: "zero length", buf: []byte{0x00}, want: []byte{}, wantPos: 1},
		{name: "length 3", buf: []byte{0x03, 0x72, 0x6f, 0x77}, want: []byte("row"), wantPos: 4},
		{name: "length runs past end", buf: []byte{0x05, 0x02, 0x03}, wantErr: true},
		{name: "empty buffer", buf: []byte{}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Decoder{buf: tt.buf}
			got, err := d.Bytes()
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}

			if tt.wantErr {
				return
			}

			if !bytes.Equal(got, tt.want) {
				t.Errorf("value: want %q, got %q", tt.want, got)
			}

			if d.pos != tt.wantPos {
				t.Errorf("pos: want %d, got %d", tt.wantPos, d.pos)
			}
		})
	}
}

func TestText(t *testing.T) {
	tests := []struct {
		name    string
		buf     []byte
		want    string
		wantPos int
		wantErr bool
	}{
		{name: "happy", buf: []byte{0x05, 0x68, 0x61, 0x70, 0x70, 0x79}, want: "happy", wantPos: 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Decoder{buf: tt.buf}
			got, err := d.Text()
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}

			if tt.wantErr {
				return
			}

			if got != tt.want {
				t.Errorf("value: want %s, got %s", tt.want, got)
			}

			if d.pos != tt.wantPos {
				t.Errorf("pos: want %d, got %d", tt.wantPos, d.pos)
			}
		})
	}
}

func TestFieldHeader(t *testing.T) {
	tests := []struct {
		name          string
		buf           []byte
		lastFieldID   int64
		wantFieldID   int64
		wantFieldType byte
		wantPos       int
		wantErr       bool
		wantErrMsg    string
	}{
		{name: "15", buf: []byte{0x15}, lastFieldID: 0, wantFieldID: 1, wantFieldType: 5, wantPos: 1},
		{name: "19", buf: []byte{0x19}, lastFieldID: 1, wantFieldID: 2, wantFieldType: 9, wantPos: 1},
		{name: "48", buf: []byte{0x48}, lastFieldID: 0, wantFieldID: 4, wantFieldType: 8, wantPos: 1},
		{name: "00", buf: []byte{0x00}, lastFieldID: 0, wantFieldID: 0, wantFieldType: 0, wantPos: 1},
		{name: "05 28", buf: []byte{0x05, 0x28}, lastFieldID: 1, wantFieldID: 20, wantFieldType: 5, wantPos: 2},
		{name: "error: 0 fieldType", buf: []byte{0x10}, wantErr: true, wantErrMsg: "invalid field type"},
		{name: "error: 14 fieldType", buf: []byte{0x1e}, wantErr: true, wantErrMsg: "invalid field type"},
		{name: "error: empty buffer", buf: []byte{}, wantErr: true, wantErrMsg: "field header"},
		{name: "error: varint", buf: []byte{0x08}, wantErr: true, wantErrMsg: "varint"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Decoder{buf: tt.buf}
			gotFieldID, gotFieldType, err := d.FieldHeader(tt.lastFieldID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}

			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("expected error message to contain '%s', but got %v", tt.wantErrMsg, err)
				}
				return
			}

			if gotFieldID != tt.wantFieldID {
				t.Errorf("fieldID: want %d, got %d", tt.wantFieldID, gotFieldID)
			}

			if gotFieldType != tt.wantFieldType {
				t.Errorf("fieldType: want %d, got %d", tt.wantFieldType, gotFieldType)
			}

			if d.pos != tt.wantPos {
				t.Errorf("pos: want %d, got %d", tt.wantPos, d.pos)
			}
		})
	}
}

func TestBool(t *testing.T) {
	tests := []struct {
		name    string
		input   byte
		want    bool
		wantPos int
		wantErr bool
	}{
		{name: "fieldType: 1", input: 0x01, want: true, wantPos: 0},
		{name: "fieldType: 2", input: 0x02, want: false, wantPos: 0},
		{name: "fieldType: 3", input: 0x03, wantErr: true, wantPos: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Decoder{}
			got, err := d.Bool(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}

			if d.pos != tt.wantPos {
				t.Errorf("pos: want %d, got: %d", tt.wantPos, d.pos)
			}

			if tt.wantErr {
				return
			}

			if got != tt.want {
				t.Errorf("value: want %t, got: %t", tt.want, got)
			}

		})
	}
}

func TestListHeader(t *testing.T) {
	tests := []struct {
		name       string
		buf        []byte
		wantSize   int
		wantType   byte
		wantPos    int
		wantErr    bool
		wantErrMsg string
	}{
		{name: "9 elements, type 12", buf: []byte{0x9c}, wantSize: 9, wantType: 0x0c, wantPos: 1},
		{name: "0 elements, type 8", buf: []byte{0x08}, wantSize: 0, wantType: 0x08, wantPos: 1},
		{name: "14 elements, type 8", buf: []byte{0xe8}, wantSize: 14, wantType: 0x08, wantPos: 1},
		{name: "15 elements, type 8", buf: []byte{0xf8, 0x0f}, wantSize: 15, wantType: 0x08, wantPos: 2},
		{name: "long form at maxInt", buf: []byte{0xf8, 0xff, 0xff, 0xff, 0xff, 0x07}, wantSize: math.MaxInt32, wantType: 0x08, wantPos: 6},
		{name: "error: 14 elements long form", buf: []byte{0xf8, 0x0e}, wantErr: true, wantErrMsg: "long form"},
		{name: "error: long form no size", buf: []byte{0xf8}, wantErr: true, wantErrMsg: "varint"},
		{name: "error: invalid type 15", buf: []byte{0x1f}, wantErr: true, wantErrMsg: "invalid element"},
		{name: "error: invalid type 0", buf: []byte{0x10}, wantErr: true, wantErrMsg: "invalid element"},
		{name: "error: empty buf", buf: []byte{}, wantErr: true, wantErrMsg: "list header"},
		{name: "long form exceeds maxInt", buf: []byte{0xf8, 0x80, 0x80, 0x80, 0x80, 0x08}, wantErr: true, wantErrMsg: "exceeds maximum"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Decoder{buf: tt.buf}
			gotSize, gotType, err := d.ListHeader()
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}

			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("expected error message to contain '%s', but got %v", tt.wantErrMsg, err)
				}
				return
			}

			if gotSize != tt.wantSize {
				t.Errorf("size: expected %d, but got %d", tt.wantSize, gotSize)
			}

			if gotType != tt.wantType {
				t.Errorf("element type: expected %d, but got %d", tt.wantType, gotType)
			}

			if d.pos != tt.wantPos {
				t.Errorf("pos: want %d, got %d", tt.wantPos, d.pos)
			}

		})
	}
}

func TestSkip(t *testing.T) {
	tests := []struct {
		name       string
		buf        []byte
		inputType  byte
		wantPos    int
		wantErr    bool
		wantErrMsg string
	}{
		{name: "boolean 1", inputType: 0x01, wantPos: 0, wantErr: false},
		{name: "boolean 2", inputType: 0x02, wantPos: 0, wantErr: false},
		{name: "i8", inputType: 0x03, buf: []byte{0x01}, wantPos: 1, wantErr: false},
		{name: "error: i8", inputType: 0x03, buf: []byte{}, wantErr: true, wantErrMsg: "insufficient"},
		{name: "i32", buf: []byte{0x01}, inputType: 0x05, wantPos: 1, wantErr: false},
		{name: "error: i32", buf: []byte{}, inputType: 0x05, wantPos: 1, wantErr: true, wantErrMsg: "varint"},
		{name: "double", buf: []byte{0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01}, inputType: 0x07, wantPos: 8, wantErr: false},
		{name: "error: double", buf: []byte{0x01}, inputType: 0x07, wantErr: true, wantErrMsg: "insufficient"},
		{name: "binary", buf: []byte{0x03, 0x72, 0x6f, 0x77}, inputType: 0x08, wantPos: 4, wantErr: false},
		{name: "error: binary", buf: []byte{}, inputType: 0x08, wantErr: true, wantErrMsg: "varint"},
		{name: "list, size 3, type 5", buf: []byte{0x35, 0x04, 0x06, 0x08}, inputType: 0x09, wantPos: 4, wantErr: false},
		{name: "error: list, invalid element type", buf: []byte{0x30}, inputType: 0x09, wantErr: true, wantErrMsg: "invalid element type"},
		{name: "struct, 1 value", buf: []byte{0x15, 0x08, 0x00}, inputType: 0x0c, wantPos: 3, wantErr: false},
		{name: "error: struct, missing stop", buf: []byte{0x15, 0x01}, inputType: 0x0c, wantErr: true, wantErrMsg: "field header"},
		{name: "uuid", buf: []byte{0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01}, inputType: 0x0d, wantPos: 16, wantErr: false},
		{name: "error: uuid", buf: []byte{0x01}, inputType: 0x0d, wantErr: true, wantErrMsg: "insufficient"},
		{name: "list of struct", buf: []byte{0x2c, 0x15, 0x04, 0x00, 0x15, 0x06, 0x00}, inputType: 0x09, wantPos: 7, wantErr: false},
		{name: "map, size 1, keytype 5, valuetype 5", buf: []byte{0x01, 0x55, 0x01, 0x01}, inputType: 0x0b, wantPos: 4, wantErr: false},
		{name: "empty map", buf: []byte{0x00}, inputType: 0x0b, wantPos: 1, wantErr: false},
		{name: "set, size 3, type 5", buf: []byte{0x35, 0x04, 0x06, 0x08}, inputType: 0x0a, wantPos: 4, wantErr: false},
		{name: "invalid type", inputType: 0x0e, wantErr: true, wantErrMsg: "invalid type"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Decoder{buf: tt.buf}
			err := d.Skip(tt.inputType)
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}

			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("expected error message to contain '%s', but got %v", tt.wantErrMsg, err)
				}
				return
			}

			if d.pos != tt.wantPos {
				t.Errorf("pos: want %d, got %d", tt.wantPos, d.pos)
			}
		})
	}
}

func TestMapHeader(t *testing.T) {
	tests := []struct {
		name          string
		buf           []byte
		wantSize      int
		wantKeyType   byte
		wantValueType byte
		wantPos       int
		wantErr       bool
		wantErrMsg    string
	}{
		{
			name:          "map with size 3, string keys and i32 values",
			buf:           []byte{0x03, 0x85},
			wantSize:      3,
			wantKeyType:   0x08,
			wantValueType: 0x05,
			wantPos:       2,
			wantErr:       false,
		},
		{
			name:          "empty map",
			buf:           []byte{0x00},
			wantSize:      0,
			wantKeyType:   0x00,
			wantValueType: 0x00,
			wantPos:       1,
			wantErr:       false,
		},
		{
			name:       "empty buffer",
			buf:        []byte{},
			wantErr:    true,
			wantErrMsg: "varint",
		},
		{
			name:          "maximum possible size",
			buf:           []byte{0xff, 0xff, 0xff, 0xff, 0x07, 0x55},
			wantSize:      math.MaxInt32,
			wantKeyType:   0x05,
			wantValueType: 0x05,
			wantPos:       6,
			wantErr:       false,
		},
		{
			name:       "size exceeds maxint",
			buf:        []byte{0x80, 0x80, 0x80, 0x80, 0x08},
			wantErr:    true,
			wantErrMsg: "exceeds",
		},
		{
			name:       "bad key type (too low)",
			buf:        []byte{0x01, 0x05},
			wantErr:    true,
			wantErrMsg: "key type",
		},
		{
			name:       "bad key type (too high)",
			buf:        []byte{0x01, 0xf5},
			wantErr:    true,
			wantErrMsg: "key type",
		},
		{
			name:       "bad value type (too low)",
			buf:        []byte{0x01, 0x50},
			wantErr:    true,
			wantErrMsg: "value type",
		},
		{
			name:       "bad value type (too high)",
			buf:        []byte{0x01, 0x5f},
			wantErr:    true,
			wantErrMsg: "value type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Decoder{buf: tt.buf}
			gotSize, gotKeyType, gotValueType, err := d.MapHeader()
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}

			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("expected error message to contain '%s', but got %v", tt.wantErrMsg, err)
				}
				return
			}

			if gotSize != tt.wantSize {
				t.Errorf("size: want %d, got %d", tt.wantSize, gotSize)
			}

			if gotKeyType != tt.wantKeyType {
				t.Errorf("keyType: want %d, got %d", tt.wantKeyType, gotKeyType)
			}

			if gotValueType != tt.wantValueType {
				t.Errorf("valueType: want %d, got %d", tt.wantValueType, gotValueType)
			}

			if d.pos != tt.wantPos {
				t.Errorf("pos: want %d, got %d", tt.wantPos, d.pos)
			}
		})
	}
}

func TestInt8(t *testing.T) {
	tests := []struct {
		name       string
		buf        []byte
		want       int8
		wantErr    bool
		wantErrMsg string
	}{
		{name: "byte present", buf: []byte{0x08}, want: 8, wantErr: false},
		{name: "empty buffer", buf: []byte{}, wantErr: true, wantErrMsg: "int8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Decoder{buf: tt.buf}
			got, err := d.Int8()
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}

			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("expected error message to contain '%s', but got %v", tt.wantErrMsg, err)
				}
				return
			}

			if got != tt.want {
				t.Errorf("int8: want %d, got %d", tt.want, got)
			}
		})
	}
}
