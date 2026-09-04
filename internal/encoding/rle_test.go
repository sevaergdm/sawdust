package encoding

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

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
			got, err := DecodeRLE(tt.input, tt.bitWidth, tt.count)
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
