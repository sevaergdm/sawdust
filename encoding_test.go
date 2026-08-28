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
		{name: "empty", input: []byte{}, want: nil, wantErr: false},
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
