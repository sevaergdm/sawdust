package sawdust

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func buildFile(fillerBytes int, claimedFooterLen uint32) []byte {
	b := []byte("PAR1")
	b = append(b, make([]byte, fillerBytes)...)

	lenField := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenField, claimedFooterLen)
	b = append(b, lenField...)

	return append(b, []byte("PAR1")...)
}

func corrupt(b []byte, i int) []byte {
	b[i] = 'X'
	return b
}

func TestReadFooter(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		want    Footer
		wantErr bool
	}{
		{
			name:  "metadata starts at the earliest legal offset",
			input: buildFile(8, 8), // 4+8+4+4 = 20 bytes, Start = 20-8-8 = 4
			want:  Footer{Start: 4, Length: 8},
		},
		{
			name:    "claimed length pushes start before the leading magic",
			input:   buildFile(8, 100),
			wantErr: true,
		},
		{
			name:    "too short to hold an envelope",
			input:   []byte("PAR1"),
			wantErr: true,
		},
		{
			name:  "normal file length",
			input: buildFile(50, 8),
			want:  Footer{Start: 46, Length: 8},
		},
		{
			name:    "bad head magic",
			input:   corrupt(buildFile(8, 8), 0),
			wantErr: true,
		},
		{
			name:    "bad tail magic",
			input:   corrupt(buildFile(8, 8), 19),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadFooter(bytes.NewReader(tt.input), int64(len(tt.input)))
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr %v, got err %v", tt.wantErr, err)
			}
			if tt.wantErr {
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("unexpected footer (-want +got):\n%s", diff)
			}
		})
	}
}
