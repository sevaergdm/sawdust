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
		{
			name:     "overshoot truncated to count",
			input:    []byte{0x1b, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa}, // 13 groups = 104 values
			bitWidth: 1,
			count:    100,
			want:     genAlt(100),
		},
		{
			name:     "width 3 straddles byte boundaries",
			input:    []byte{0x03, 0x88, 0xc6, 0xfa},
			bitWidth: 3,
			count:    8,
			want:     []int64{0, 1, 2, 3, 4, 5, 6, 7},
		},
		{
			name:       "error: empty buffer",
			input:      []byte{},
			bitWidth:   1,
			count:      10,
			wantErr:    true,
			wantErrMsg: "varint is truncated",
		},
		{
			name:       "error: bit-packed run truncated",
			input:      []byte{0x19, 0xaa}, // header claims 12 groups, 1 byte follows
			bitWidth:   1,
			count:      100,
			wantErr:    true,
			wantErrMsg: "run needs 12 bytes",
		},
		{
			name:       "error: RLE run missing its value",
			input:      []byte{0x02}, // RLE header, no value byte
			bitWidth:   1,
			count:      10,
			wantErr:    true,
			wantErrMsg: "only 0 remain",
		},
		{
			name:       "error: RLE value exceeds bit width",
			input:      []byte{0x02, 0x07}, // value 7 in a 1-bit column
			bitWidth:   1,
			count:      4,
			wantErr:    true,
			wantErrMsg: "exceeds largest value possible",
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

func TestApplyLevels(t *testing.T) {
	tests := []struct {
		name        string
		defLevels   []int64
		repLevels   []int64
		values      []int64
		maxDefLevel int64
		maxRepLevel int64
		numRows     int
		want        []*int64
		wantOffsets []int
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name:        "basic int64, alternating nils",
			defLevels:   []int64{1, 0, 1, 0, 1, 0, 1, 0, 1, 0},
			values:      []int64{5, 4, 3, 2, 1},
			maxDefLevel: 1,
			numRows:     10,
			want:        []*int64{ptr(int64(5)), nil, ptr(int64(4)), nil, ptr(int64(3)), nil, ptr(int64(2)), nil, ptr(int64(1)), nil},
			wantOffsets: seqOffsets(10),
			wantErr:     false,
		},
		{
			name:        "all nulls",
			defLevels:   []int64{0, 0, 0, 0, 0},
			values:      []int64{},
			maxDefLevel: 1,
			numRows:     5,
			want:        []*int64{nil, nil, nil, nil, nil},
			wantOffsets: seqOffsets(5),
			wantErr:     false,
		},
		{
			name:        "no nulls",
			defLevels:   []int64{1, 1, 1, 1, 1},
			values:      []int64{5, 4, 3, 2, 1},
			maxDefLevel: 1,
			numRows:     5,
			wantOffsets: seqOffsets(5),
			want:        []*int64{ptr(int64(5)), ptr(int64(4)), ptr(int64(3)), ptr(int64(2)), ptr(int64(1))},
			wantErr:     false,
		},
		{
			name:        "nested: intermediate level is null",
			defLevels:   []int64{2, 1, 0, 2},
			values:      []int64{10, 20},
			maxDefLevel: 2,
			numRows:     4,
			wantOffsets: seqOffsets(4),
			want:        []*int64{ptr(int64(10)), nil, nil, ptr(int64(20))},
			wantErr:     false,
		},
		{
			name:        "error: cursor exceeds",
			defLevels:   []int64{1, 1, 1, 1, 1, 1},
			values:      []int64{1, 2, 3, 4},
			maxDefLevel: 1,
			numRows:     6,
			wantErr:     true,
			wantErrMsg:  "only 4 were stored",
		},
		{
			name:        "error: values left unconsumed",
			defLevels:   []int64{1, 0, 1, 1},
			values:      []int64{1, 2, 3, 4},
			maxDefLevel: 1,
			numRows:     4,
			wantErr:     true,
			wantErrMsg:  "values were present",
		},
		{
			name:        "repeated: tags shape",
			repLevels:   []int64{0, 0, 1, 0, 0},
			defLevels:   []int64{1, 1, 1, 0, 1},
			values:      []int64{10, 20, 30, 40},
			maxDefLevel: 1,
			maxRepLevel: 1,
			numRows:     4,
			want:        []*int64{ptr(int64(10)), ptr(int64(20)), ptr(int64(30)), ptr(int64(40))},
			wantOffsets: []int{0, 1, 3, 3, 4},
		},
		{
			name:        "repeated: every list empty",
			repLevels:   []int64{0, 0, 0},
			defLevels:   []int64{0, 0, 0},
			values:      []int64{},
			maxDefLevel: 1,
			maxRepLevel: 1,
			numRows:     3,
			want:        nil,
			wantOffsets: []int{0, 0, 0, 0},
		},
		{
			name:        "repeated: one row three values",
			repLevels:   []int64{0, 1, 1},
			defLevels:   []int64{1, 1, 1},
			values:      []int64{1, 2, 3},
			maxDefLevel: 1,
			maxRepLevel: 1,
			numRows:     1,
			want:        []*int64{ptr(int64(1)), ptr(int64(2)), ptr(int64(3))},
			wantOffsets: []int{0, 3},
		},
		{
			name:        "error: nesting deeper than one level",
			repLevels:   []int64{0},
			defLevels:   []int64{0},
			values:      []int64{},
			maxDefLevel: 2,
			maxRepLevel: 2,
			numRows:     1,
			wantErr:     true,
			wantErrMsg:  "nesting deeper than one level",
		},

		{
			name:        "error: nullable elements in a list",
			repLevels:   []int64{0},
			defLevels:   []int64{2},
			values:      []int64{1},
			maxDefLevel: 2,
			maxRepLevel: 1,
			numRows:     1,
			wantErr:     true,
			wantErrMsg:  "nullable elements",
		},

		{
			name:        "error: level streams differ in length",
			repLevels:   []int64{0},
			defLevels:   []int64{1, 1},
			values:      []int64{1, 2},
			maxDefLevel: 1,
			maxRepLevel: 1,
			numRows:     2,
			wantErr:     true,
			wantErrMsg:  "should match",
		},

		{
			name:        "error: row count disagrees with header",
			repLevels:   []int64{0, 0},
			defLevels:   []int64{1, 1},
			values:      []int64{1, 2},
			maxDefLevel: 1,
			maxRepLevel: 1,
			numRows:     5,
			wantErr:     true,
			wantErrMsg:  "header says 5",
		},
		{
			name:        "flat: required column",
			defLevels:   []int64{0, 0, 0},
			values:      []int64{7, 8, 9},
			maxDefLevel: 0,
			numRows:     3,
			want:        []*int64{ptr(int64(7)), ptr(int64(8)), ptr(int64(9))},
			wantOffsets: seqOffsets(3),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotColumnValues, gotOffsets, err := applyLevels(tt.repLevels, tt.defLevels, tt.values, tt.maxDefLevel, tt.maxRepLevel, tt.numRows)
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}

			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("expected error to contain %q, but got: %v", tt.wantErrMsg, err)
				}
				return
			}

			if diff := cmp.Diff(tt.want, gotColumnValues); diff != "" {
				t.Errorf("mismatch column values (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(tt.wantOffsets, gotOffsets); diff != "" {
				t.Errorf("mismatch offsets (-want +got):\n%s", diff)
			}
		})
	}
}

func TestApplyLevelsString(t *testing.T) {
	defLevels := []int64{1, 0, 1, 1, 0}
	repLevels := []int64{0, 0, 0, 0, 0}
	values := []string{"a", "b", "c"}
	want := []*string{ptr(string("a")), nil, ptr(string("b")), ptr(string("c")), nil}
	wantOffsets := seqOffsets(5)

	gotColumnValues, gotOffsets, err := applyLevels(repLevels, defLevels, values, 1, 0, 5)
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	if diff := cmp.Diff(want, gotColumnValues); diff != "" {
		t.Errorf("mismatch column values (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(wantOffsets, gotOffsets); diff != "" {
		t.Errorf("mismatch offsets (-want +got):\n%s", diff)
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

func seqOffsets(n int) []int {
	out := make([]int, 0, n+1)
	for i := range n + 1 {
		out = append(out, i)
	}
	return out
}
