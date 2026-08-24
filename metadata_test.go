package sawdust

import (
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const genCreatedBy = "github.com/parquet-go/parquet-go version 0.32.0(build )"

func strPtr(s string) *string { return &s }

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

var basicSchema = []SchemaElement{
	{Name: "row", NumChildren: ptr(int64(8))},
	{Name: "row_number", Type: ptr(TypeInt64), TypeLength: ptr(int64(64)), RepetitionType: ptr(RepetitionRequired)},
	{Name: "even_row_number", Type: ptr(TypeInt64), TypeLength: ptr(int64(64)), RepetitionType: ptr(RepetitionOptional)},
	{Name: "rand_id", Type: ptr(TypeByteArray), RepetitionType: ptr(RepetitionRequired)},
	{Name: "opt_rand_id", Type: ptr(TypeByteArray), RepetitionType: ptr(RepetitionOptional)},
	{Name: "category", Type: ptr(TypeByteArray), RepetitionType: ptr(RepetitionRequired)},
	{Name: "rand_float", Type: ptr(TypeDouble), TypeLength: ptr(int64(64)), RepetitionType: ptr(RepetitionRequired)},
	{Name: "ts", Type: ptr(TypeInt64), TypeLength: ptr(int64(64)), RepetitionType: ptr(RepetitionRequired)},
	{Name: "is_odd", Type: ptr(TypeBoolean), TypeLength: ptr(int64(1)), RepetitionType: ptr(RepetitionRequired)},
}

var nestedSchema = []SchemaElement{
	{Name: "nestedRow", NumChildren: ptr(int64(4))},
	{Name: "id", Type: ptr(TypeInt64), TypeLength: ptr(int64(64)), RepetitionType: ptr(RepetitionRequired)},
	{Name: "inner", NumChildren: ptr(int64(2)), RepetitionType: ptr(RepetitionRequired)},
	{Name: "a", Type: ptr(TypeInt64), TypeLength: ptr(int64(64)), RepetitionType: ptr(RepetitionRequired)},
	{Name: "b", Type: ptr(TypeByteArray), RepetitionType: ptr(RepetitionRequired)},
	{Name: "opt_in", NumChildren: ptr(int64(2)), RepetitionType: ptr(RepetitionOptional)},
	{Name: "a", Type: ptr(TypeInt64), TypeLength: ptr(int64(64)), RepetitionType: ptr(RepetitionRequired)},
	{Name: "b", Type: ptr(TypeByteArray), RepetitionType: ptr(RepetitionRequired)},
	{Name: "tags", Type: ptr(TypeByteArray), RepetitionType: ptr(RepetitionRepeated)},
}

func TestBuildTree(t *testing.T) {
	root, err := BuildTree(nestedSchema)
	if err != nil {
		t.Fatalf("unexpected error building tree: %v", err)
	}

	wantSchemaNode := SchemaNode{
		Element: nestedSchema[0],
		Children: []SchemaNode{
			{Element: nestedSchema[1]},
			{Element: nestedSchema[2], Children: []SchemaNode{
				{Element: nestedSchema[3]},
				{Element: nestedSchema[4]},
			}},
			{Element: nestedSchema[5], Children: []SchemaNode{
				{Element: nestedSchema[6]},
				{Element: nestedSchema[7]},
			}},
			{Element: nestedSchema[8]},
		},
	}

	if diff := cmp.Diff(wantSchemaNode, root); diff != "" {
		t.Errorf("tree mismatch (-want +got):\n%s", diff)
	}
}

func TestReadFileMetadataRealFixtures(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		wantVersion   int64
		wantNumRows   int64
		wantCreatedBy string
		wantSchema    []SchemaElement
	}{
		{name: "basic", path: "testdata/basic.parquet", wantVersion: 2, wantNumRows: 100, wantCreatedBy: genCreatedBy, wantSchema: basicSchema},
		{name: "empty", path: "testdata/empty.parquet", wantVersion: 2, wantNumRows: 0, wantCreatedBy: genCreatedBy, wantSchema: basicSchema},
		{name: "many_rows", path: "testdata/many_rows.parquet", wantVersion: 2, wantNumRows: 300, wantCreatedBy: genCreatedBy, wantSchema: basicSchema},
		{name: "single_row", path: "testdata/single_row.parquet", wantVersion: 2, wantNumRows: 1, wantCreatedBy: genCreatedBy, wantSchema: basicSchema},
		{name: "nested", path: "testdata/nested.parquet", wantVersion: 2, wantNumRows: 100, wantCreatedBy: genCreatedBy, wantSchema: nestedSchema},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.Open(tt.path)
			if err != nil {
				t.Fatalf("unable to open %s: %v", tt.path, err)
			}
			defer func() { _ = f.Close() }()

			fStat, err := f.Stat()
			if err != nil {
				t.Fatalf("could not stat %s: %v", tt.path, err)
			}

			size := fStat.Size()

			footer, err := ReadFooter(f, size)
			if err != nil {
				t.Fatalf("encountered an error reading footer in %s: %v", tt.path, err)
			}

			footerBytes := make([]byte, footer.Length)
			_, err = f.ReadAt(footerBytes, footer.Start)
			if err != nil {
				t.Fatalf("unable to read footer at offset %d: %v", footer.Start, err)
			}

			fileMetadata, err := ReadFileMetadata(footerBytes)
			if err != nil {
				t.Fatalf("unexpected error reading file metadata: %v", err)
			}

			if fileMetadata.Version != tt.wantVersion {
				t.Errorf("version: want %d, got %d", tt.wantVersion, fileMetadata.Version)
			}

			if fileMetadata.NumRows != tt.wantNumRows {
				t.Errorf("num_rows: want %d, got %d", tt.wantNumRows, fileMetadata.NumRows)
			}

			switch {
			case fileMetadata.CreatedBy == nil:
				t.Errorf("created_by: want %q, got nil", tt.wantCreatedBy)
			case *fileMetadata.CreatedBy != tt.wantCreatedBy:
				t.Errorf("created_by: want %s, got %s", tt.wantCreatedBy, *fileMetadata.CreatedBy)
			}

			if diff := cmp.Diff(tt.wantSchema, fileMetadata.Schema); diff != "" {
				t.Errorf("schema mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReadFileMetadata(t *testing.T) {
	schemaField := []byte{0x19, 0x1c, 0x48, 0x01, 0x72, 0x15, 0x00, 0x00}
	wantSchema := []SchemaElement{{Name: "r", NumChildren: ptr(int64(0))}}
	tests := []struct {
		name          string
		input         []byte
		wantVersion   int64
		wantNumRows   int64
		wantCreatedBy *string
		wantSchema    []SchemaElement
		wantErr       bool
		wantErrMsg    string
	}{
		{
			name:        "version 2, num_rows 100, created_by nil",
			input:       cat([]byte{0x15, 0x04}, schemaField, []byte{0x16, 0xc8, 0x01}, []byte{0x00}),
			wantVersion: 2,
			wantNumRows: 100,
			wantSchema:  wantSchema,
			wantErr:     false,
		},
		{
			name:       "error: missing num_rows",
			input:      []byte{0x15, 0x04, 0x00},
			wantErr:    true,
			wantErrMsg: "num_rows",
		},
		{
			name:        "version 2, num_rows 100, created_by nil, skip unknown",
			input:       cat([]byte{0x15, 0x04}, schemaField, []byte{0x16, 0xc8, 0x01}, []byte{0x05, 0x28, 0x02}, []byte{0x00}),
			wantVersion: 2,
			wantNumRows: 100,
			wantSchema:  wantSchema,
			wantErr:     false,
		},
		{
			name:       "error: malformed",
			input:      []byte{0x15, 0x04, 0x26, 0xc8, 0x01},
			wantErr:    true,
			wantErrMsg: "malformed",
		},
		{
			name:          "version 2, num_rows 100, created_by 'x'",
			input:         cat([]byte{0x15, 0x04}, schemaField, []byte{0x16, 0xc8, 0x01}, []byte{0x38, 0x01, 0x78}, []byte{0x00}),
			wantVersion:   2,
			wantNumRows:   100,
			wantCreatedBy: strPtr("x"),
			wantSchema:    wantSchema,
			wantErr:       false,
		},
		{
			name:       "error: missing schema",
			input:      cat([]byte{0x15, 0x04}, []byte{0x26, 0xc8, 0x01}, []byte{0x00}),
			wantErr:    true,
			wantErrMsg: "schema",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadFileMetadata(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}

			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("expected error message to contain '%s', but got %v", tt.wantErrMsg, err)
				}
				return
			}

			if got.Version != tt.wantVersion {
				t.Errorf("version: want %d, got %d", tt.wantVersion, got.Version)
			}

			if got.NumRows != tt.wantNumRows {
				t.Errorf("num_rows: want %d, got %d", tt.wantNumRows, got.NumRows)
			}

			if diff := cmp.Diff(tt.wantCreatedBy, got.CreatedBy); diff != "" {
				t.Errorf("created_by mismatch (-want +got):\n%s", diff)
			}

		})
	}
}

func FuzzReadFileMetadata(f *testing.F) {
	f.Add([]byte{0x15, 0x04, 0x26, 0xc8, 0x01, 0x00})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ReadFileMetadata(data)
	})
}
