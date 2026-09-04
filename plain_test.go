package sawdust

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDecodePlainByteArray(t *testing.T) {
	tests := []struct {
		name       string
		input      []byte
		want       [][]byte
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:    "foo",
			input:   []byte{3, 0, 0, 0, 'f', 'o', 'o'},
			want:    [][]byte{[]byte("foo")},
			wantErr: false,
		},
		{
			name:    "foo bar baz buzz",
			input:   []byte{3, 0, 0, 0, 'f', 'o', 'o', 3, 0, 0, 0, 'b', 'a', 'r', 3, 0, 0, 0, 'b', 'a', 'z', 4, 0, 0, 0, 'b', 'u', 'z', 'z'},
			want:    [][]byte{[]byte("foo"), []byte("bar"), []byte("baz"), []byte("buzz")},
			wantErr: false,
		},
		{
			name:    "zero length, no error",
			input:   []byte{0, 0, 0, 0},
			want:    [][]byte{{}},
			wantErr: false,
		},
		{
			name:    "empty, no error",
			input:   nil,
			want:    nil,
			wantErr: false,
		},
		{
			name:       "error: trailing bytes",
			input:      []byte{3, 0, 0, 0, 'a', 'b', 'c', 9, 9},
			wantErr:    true,
			wantErrMsg: "malformed",
		},
		{
			name:       "error: overrunning length",
			input:      []byte{9, 0, 0, 0, 'b', 'a', 'r'},
			wantErr:    true,
			wantErrMsg: "declared length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodePlainByteArray(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}

			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("expected error to contain %q, but got: %v", tt.wantErrMsg, err)
				}
				return
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}

		})
	}
}

func TestDecodePlainInt64(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		want    []int64
		wantErr bool
	}{
		{name: "1", input: []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, want: []int64{1}, wantErr: false},
		{name: "5, 8", input: []byte{0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, want: []int64{5, 8}, wantErr: false},
		{name: "empty", input: []byte{}, want: []int64{}, wantErr: false},
		{name: "error", input: []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, wantErr: true},
		{name: "-1", input: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, want: []int64{-1}, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodePlainInt64(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}

			if tt.wantErr {
				return
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDecodePlainDouble(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		want    []float64
		wantErr bool
	}{
		{name: "1", input: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f}, want: []float64{1}, wantErr: false},
		{name: "-2.25", input: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xc0}, want: []float64{-2.25}, wantErr: false},
		{name: "error: 7 bytes", input: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodePlainDouble(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}

			if tt.wantErr {
				return
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDecodePlainBoolean(t *testing.T) {
	tests := []struct {
		name       string
		input      []byte
		count      int
		want       []bool
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:    "4 vals, alternating",
			input:   []byte{0x0a},
			want:    []bool{false, true, false, true},
			count:   4,
			wantErr: false,
		},
		{
			name:    "0 count, no error",
			input:   []byte{},
			want:    []bool{},
			count:   0,
			wantErr: false,
		},
		{
			name:    "values less than 1 byte",
			input:   []byte{0x01},
			count:   5,
			want:    []bool{true, false, false, false, false},
			wantErr: false,
		},
		{
			name:       "negative count",
			input:      []byte{},
			count:      -1,
			wantErr:    true,
			wantErrMsg: "negative",
		},
		{
			name:       "count exceeds length, error",
			input:      []byte{0x01},
			count:      12,
			wantErr:    true,
			wantErrMsg: "requires",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodePlainBoolean(tt.input, tt.count)
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}

			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("expected error to contain %q, but got: %v", tt.wantErrMsg, err)
				}
				return
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
