package sawdust

import (
	"bytes"
	"encoding/binary"
	"strings"
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

func TestReadPagesRealFixtures(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		hasChunks        bool
		requireMultiPage bool
	}{
		{name: "basic", path: "testdata/basic.parquet", hasChunks: true, requireMultiPage: false},
		{name: "empty", path: "testdata/empty.parquet", hasChunks: false, requireMultiPage: false},
		{name: "many_rows", path: "testdata/many_rows.parquet", hasChunks: true, requireMultiPage: false},
		{name: "single_row", path: "testdata/single_row.parquet", hasChunks: true, requireMultiPage: false},
		{name: "nested", path: "testdata/nested.parquet", hasChunks: true, requireMultiPage: false},
		{name: "zstd", path: "testdata/zstd.parquet", hasChunks: true, requireMultiPage: false},
		{name: "multi_page", path: "testdata/multi_page.parquet", hasChunks: true, requireMultiPage: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := OpenFile(tt.path)
			if err != nil {
				t.Fatalf("OpenFile: %v", err)
			}
			defer func() { _ = f.Close() }()
			multiPageSeen := false
			numChunks := 0

			rowGroups := f.Metadata.RowGroups

			for _, group := range rowGroups {
				for _, c := range group.Columns {
					pages, err := f.ReadPages(c)
					if err != nil {
						t.Errorf("expected no error, but got: %v", err)
						continue
					}
					numChunks++

					for _, page := range pages {
						if len(page.Data) != int(page.Header.CompressedPageSize) {
							t.Errorf("page size: want %d, got %d", page.Header.CompressedPageSize, len(page.Data))
						}
					}

					if len(pages) > 1 {
						multiPageSeen = true
					}
				}
			}

			if (numChunks > 0) != tt.hasChunks {
				t.Errorf("expected hasChunks %t, but got %d chunks", tt.hasChunks, numChunks)
			}

			if tt.requireMultiPage && !multiPageSeen {
				t.Errorf("expected requireMultiPage %t, but got %t", tt.requireMultiPage, multiPageSeen)
			}
		})
	}
}

func TestReadPagesErr(t *testing.T) {
	tests := []struct {
		name       string
		file       *File
		chunk      ColumnChunk
		wantErrMsg string
	}{
		{
			name:       "nil metadata",
			file:       &File{Size: 100},
			chunk:      ColumnChunk{},
			wantErrMsg: "no page data",
		},
		{
			name:       "negative size",
			file:       &File{Size: 100},
			chunk:      ColumnChunk{Metadata: &ColumnMetadata{TotalCompressedSize: -1}},
			wantErrMsg: "malformed payload size",
		},
		{
			name:       "size larger than file",
			file:       &File{Size: 100},
			chunk:      ColumnChunk{Metadata: &ColumnMetadata{TotalCompressedSize: 101}},
			wantErrMsg: "malformed payload size",
		},
		{
			name:       "reader too short",
			file:       &File{reader: bytes.NewReader([]byte{}), Size: 10},
			chunk:      ColumnChunk{Metadata: &ColumnMetadata{TotalCompressedSize: 10}},
			wantErrMsg: "error reading file at 0",
		},
		{
			name:       "garbage header",
			file:       &File{reader: bytes.NewReader([]byte{0xff, 0xff, 0xff, 0xff}), Size: 4},
			chunk:      ColumnChunk{Metadata: &ColumnMetadata{TotalCompressedSize: 4}},
			wantErrMsg: "invalid field type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.file.ReadPages(tt.chunk)
			if err == nil {
				t.Fatalf("want error containing %q, but got none", tt.wantErrMsg)
			}

			if !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("want error containing %q, but got %v", tt.wantErrMsg, err)
			}
		})
	}
}

func TestReadInt64Column(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		column     string
		want       []int64
		wantErr    bool
		wantErrMsg string
	}{
		{name: "basic", path: "testdata/basic.parquet", column: "row_number", want: genInt64(100), wantErr: false},
		{name: "zstd", path: "testdata/zstd.parquet", column: "row_number", want: genInt64(100), wantErr: false},
		{name: "many_rows", path: "testdata/many_rows.parquet", column: "row_number", want: genInt64(300), wantErr: false},
		{name: "single_row", path: "testdata/single_row.parquet", column: "row_number", want: genInt64(1), wantErr: false},
		{name: "empty", path: "testdata/empty.parquet", column: "row_number", want: genInt64(0), wantErr: false},
		{name: "wrong type", path: "testdata/basic.parquet", column: "category", wantErr: true, wantErrMsg: "not an int64"},
		{name: "missing column", path: "testdata/basic.parquet", column: "fake", wantErr: true, wantErrMsg: "not found in schema"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := OpenFile(tt.path)
			if err != nil {
				t.Fatalf("OpenFile: %v", err)
			}
			defer func() { _ = f.Close() }()

			got, err := f.ReadInt64Column(tt.column)
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr %v, got err %v", tt.wantErr, err)
			}

			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("expected error message to contain '%s', but got %v", tt.wantErrMsg, err)
				}
				return
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReadInt64ColumnBasicZstd(t *testing.T) {
	basic, err := OpenFile("testdata/basic.parquet")
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer func() { _ = basic.Close() }()

	zstd, err := OpenFile("testdata/zstd.parquet")
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer func() { _ = zstd.Close() }()

	gotBasic, err := basic.ReadInt64Column("row_number")
	if err != nil {
		t.Fatalf("wanted no error, but got: %v", err)
	}

	gotZstd, err := zstd.ReadInt64Column("row_number")
	if err != nil {
		t.Fatalf("wanted no error, but got: %v", err)
	}

	if diff := cmp.Diff(gotBasic, gotZstd); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func genInt64(limit int) []int64 {
	if limit == 0 {
		return nil
	}
	output := make([]int64, 0, limit)
	for i := 1; i <= limit; i++ {
		output = append(output, int64(i))
	}
	return output
}
