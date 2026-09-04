package sawdust

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDecodeDeltaBinary(t *testing.T) {
	tests := []struct {
		name         string
		input        []byte
		want         []int64
		wantConsumed int
		wantErr      bool
		wantErrMsg   string
	}{
		{
			// header only — total is 1, so firstValue is the whole answer and no block is read
			name:         "single value",
			input:        []byte{0x80, 0x01, 0x04, 0x01, 0x0a},
			want:         []int64{5},
			wantConsumed: 5,
		},
		{
			// one block, minDelta 3 (zigzag 0x06), all four bit widths zero
			name:         "bit width 0",
			input:        []byte{0x80, 0x01, 0x04, 0x02, 0x0a, 0x06, 0x00, 0x00, 0x00, 0x00},
			want:         []int64{5, 8},
			wantConsumed: 10,
		},
		{
			// same bytes, total 5 — proves truncation, since the block produces 128 values
			name:         "bit width 0,  five values",
			input:        []byte{0x80, 0x01, 0x04, 0x05, 0x0a, 0x06, 0x00, 0x00, 0x00, 0x00},
			want:         []int64{5, 8, 11, 14, 17},
			wantConsumed: 10,
		},
		{
			// negative first value and negative minDelta
			name:         "negatives",
			input:        []byte{0x80, 0x01, 0x04, 0x03, 0x09, 0x03, 0x00, 0x00, 0x00, 0x00},
			want:         []int64{-5, -7, -9},
			wantConsumed: 10,
		},
		{
			name:       "error: blockSize not multiple of 128",
			input:      []byte{0x40, 0x04, 0x01, 0x0a},
			wantErr:    true,
			wantErrMsg: "multiple of 128",
		},
		{
			name:       "error: zero miniblocks",
			input:      []byte{0x80, 0x01, 0x00, 0x01, 0x0a},
			wantErr:    true,
			wantErrMsg: "cannot be 0",
		},
		{
			name:       "truncated header",
			input:      []byte{0x80, 0x01},
			wantErr:    true,
			wantErrMsg: "varint is truncated",
		},
		{
			name:       "truncated bit widths",
			input:      []byte{0x80, 0x01, 0x04, 0x02, 0x0a, 0x06, 0x00},
			wantErr:    true,
			wantErrMsg: "exceeds total available bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVals, gotConsumed, err := decodeDeltaBinary(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}
			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("expected error to contain %q, but got: %v", tt.wantErrMsg, err)
				}
				return
			}

			if diff := cmp.Diff(tt.want, gotVals); diff != "" {
				t.Errorf("mismatch (-want +got):%s\n", diff)
			}

			if tt.wantConsumed != gotConsumed {
				t.Errorf("want %d consumed, but got %d", tt.wantConsumed, gotConsumed)
			}
		})
	}
}

func TestDecodeDeltaBinaryRealCase(t *testing.T) {
	input := []byte{
		0x80, 0x01, 0x04, 0x64, 0x06, 0x05, 0x03, 0x03, 0x03, 0x03, 0xa3, 0x36,
		0x6a, 0xd4, 0x36, 0x8a, 0xda, 0xa8, 0x6d, 0xdb, 0xb6, 0x71, 0xda, 0xb6,
		0x71, 0x1a, 0x27, 0x4e, 0xdb, 0xb6, 0x8d, 0xd3, 0x36, 0x8a, 0xda, 0x38,
		0x71, 0xda, 0x38, 0x51, 0xdb, 0xa8, 0x8d, 0xda, 0xb6, 0x6d, 0xa3, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}

	wantFirst8 := []int64{3, 3, 4, 3, 3, 3, 4, 3}
	wantLast4 := []int64{3, 3, 4, 3}

	gotVals, gotConsumed, err := decodeDeltaBinary(input)
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	if gotConsumed != len(input) {
		t.Errorf("expected %d consumed, but got %d", len(input), gotConsumed)
	}

	total := int64(0)
	for _, v := range gotVals {
		total += v
	}

	if total != 328 {
		t.Errorf("expected sum of returned vals to be 328, but got %d", total)
	}

	if diff := cmp.Diff(wantFirst8, gotVals[:8]); diff != "" {
		t.Errorf("mismatch first 8 vals (+want -got):%s\n", diff)
	}

	if diff := cmp.Diff(wantLast4, gotVals[len(gotVals)-4:]); diff != "" {
		t.Errorf("mismatch last 4 vals (+want -got):%s\n", diff)
	}
}
