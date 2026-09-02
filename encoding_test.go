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

func TestDecodeRLE(t *testing.T) {
	tests := []struct {
		name       string
		input      []byte
		bitWidth   int
		count      int
		want       []int64
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "100 alternating",
			input: []byte{0x19, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa,
				0x02, 0x00, 0x02, 0x01, 0x02, 0x00, 0x02, 0x01},
			bitWidth: 1,
			count:    100,
			want:     genAlt(100),
			wantErr:  false,
		},
		{
			name:     "RLE + bitpacked",
			input:    []byte{0x08, 0x02, 0x03, 0xe4, 0x1b},
			bitWidth: 2,
			count:    12,
			want:     []int64{2, 2, 2, 2, 0, 1, 2, 3, 3, 2, 1, 0},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeRLE(tt.input, tt.bitWidth, tt.count)
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

func TestApplyDefinitionLevels(t *testing.T) {
	tests := []struct {
		name        string
		defLevels   []int64
		values      []int64
		maxDefLevel int64
		want        []*int64
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name:        "basic int64, alternating nils",
			defLevels:   []int64{1, 0, 1, 0, 1, 0, 1, 0, 1, 0},
			values:      []int64{5, 4, 3, 2, 1},
			maxDefLevel: 1,
			want:        []*int64{ptr(int64(5)), nil, ptr(int64(4)), nil, ptr(int64(3)), nil, ptr(int64(2)), nil, ptr(int64(1)), nil},
			wantErr:     false,
		},
		{
			name:        "all nulls",
			defLevels:   []int64{0, 0, 0, 0, 0},
			values:      []int64{},
			maxDefLevel: 1,
			want:        []*int64{nil, nil, nil, nil, nil},
			wantErr:     false,
		},
		{
			name:        "no nulls",
			defLevels:   []int64{1, 1, 1, 1, 1},
			values:      []int64{5, 4, 3, 2, 1},
			maxDefLevel: 1,
			want:        []*int64{ptr(int64(5)), ptr(int64(4)), ptr(int64(3)), ptr(int64(2)), ptr(int64(1))},
			wantErr:     false,
		},
		{
			name:        "nested: intermediate level is null",
			defLevels:   []int64{2, 1, 0, 2},
			values:      []int64{10, 20},
			maxDefLevel: 2,
			want:        []*int64{ptr(int64(10)), nil, nil, ptr(int64(20))},
			wantErr:     false,
		},
		{
			name:        "error: cursor exceeds",
			defLevels:   []int64{1, 1, 1, 1, 1, 1},
			values:      []int64{1, 2, 3, 4},
			maxDefLevel: 1,
			wantErr:     true,
			wantErrMsg:  "exceeds available values",
		},
		{
			name:        "error: values left unconsumed",
			defLevels:   []int64{1, 0, 1, 1},
			values:      []int64{1, 2, 3, 4},
			maxDefLevel: 1,
			wantErr:     true,
			wantErrMsg:  "values were present",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyDefinitionLevels(tt.defLevels, tt.values, tt.maxDefLevel)
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

func TestApplyDefinitionLevelsString(t *testing.T) {
	defLevels := []int64{1, 0, 1, 1, 0}
	values := []string{"a", "b", "c"}
	want := []*string{ptr(string("a")), nil, ptr(string("b")), ptr(string("c")), nil}

	got, err := applyDefinitionLevels(defLevels, values, 1)
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func genAlt(n int) []int64 {
	out := make([]int64, 0, n)
	for i := range n {
		if i%2 == 0 {
			out = append(out, 0)
		} else {
			out = append(out, 1)
		}
	}
	return out
}
