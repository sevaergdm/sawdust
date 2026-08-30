package sawdust

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

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

func TestDecodeRLE(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		bitWidth int
		count    int
		want     []int32
	}{
		{
			name: "100 alternating",
			input: []byte{0x19, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa,
				0x02, 0x00, 0x02, 0x01, 0x02, 0x00, 0x02, 0x01},
			bitWidth: 1,
			count:    100,
			want:     genAlt(100),
		},
		{
			name:     "RLE + bitpacked",
			input:    []byte{0x08, 0x02, 0x03, 0xe4, 0x1b},
			bitWidth: 2,
			count:    12,
			want:     []int32{2, 2, 2, 2, 0, 1, 2, 3, 3, 2, 1, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeRLE(tt.input, tt.bitWidth, tt.count)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}

		})
	}

}

func genAlt(n int) []int32 {
	out := make([]int32, 0, n)
	for i := range n {
		if i%2 == 0 {
			out = append(out, 0)
		} else {
			out = append(out, 1)
		}
	}
	return out
}
