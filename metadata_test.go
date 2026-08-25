package sawdust

import (
	"encoding/binary"
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
	{Name: "row_number", Type: ptr(TypeInt64), TypeLength: ptr(int64(64)), RepetitionType: ptr(RepetitionRequired), ConvertedType: ptr(ConvertedInt64), LogicalType: IntType{BitWidth: 64, IsSigned: true}},
	{Name: "even_row_number", Type: ptr(TypeInt64), TypeLength: ptr(int64(64)), RepetitionType: ptr(RepetitionOptional), ConvertedType: ptr(ConvertedInt64), LogicalType: IntType{BitWidth: 64, IsSigned: true}},
	{Name: "rand_id", Type: ptr(TypeByteArray), RepetitionType: ptr(RepetitionRequired), ConvertedType: ptr(ConvertedUTF8), LogicalType: StringType{}},
	{Name: "opt_rand_id", Type: ptr(TypeByteArray), RepetitionType: ptr(RepetitionOptional), ConvertedType: ptr(ConvertedUTF8), LogicalType: StringType{}},
	{Name: "category", Type: ptr(TypeByteArray), RepetitionType: ptr(RepetitionRequired), ConvertedType: ptr(ConvertedUTF8), LogicalType: StringType{}},
	{Name: "rand_float", Type: ptr(TypeDouble), TypeLength: ptr(int64(64)), RepetitionType: ptr(RepetitionRequired)},
	{Name: "ts", Type: ptr(TypeInt64), TypeLength: ptr(int64(64)), RepetitionType: ptr(RepetitionRequired), ConvertedType: ptr(ConvertedTimestampMicros), LogicalType: TimestampType{IsAdjustedToUTC: true, Unit: TimeMicros}},
	{Name: "is_odd", Type: ptr(TypeBoolean), TypeLength: ptr(int64(1)), RepetitionType: ptr(RepetitionRequired)},
}

var nestedSchema = []SchemaElement{
	{Name: "nestedRow", NumChildren: ptr(int64(4))},
	{Name: "id", Type: ptr(TypeInt64), TypeLength: ptr(int64(64)), RepetitionType: ptr(RepetitionRequired), ConvertedType: ptr(ConvertedInt64), LogicalType: IntType{BitWidth: 64, IsSigned: true}},
	{Name: "inner", NumChildren: ptr(int64(2)), RepetitionType: ptr(RepetitionRequired)},
	{Name: "a", Type: ptr(TypeInt64), TypeLength: ptr(int64(64)), RepetitionType: ptr(RepetitionRequired), ConvertedType: ptr(ConvertedInt64), LogicalType: IntType{BitWidth: 64, IsSigned: true}},
	{Name: "b", Type: ptr(TypeByteArray), RepetitionType: ptr(RepetitionRequired), ConvertedType: ptr(ConvertedUTF8), LogicalType: StringType{}},
	{Name: "opt_in", NumChildren: ptr(int64(2)), RepetitionType: ptr(RepetitionOptional)},
	{Name: "a", Type: ptr(TypeInt64), TypeLength: ptr(int64(64)), RepetitionType: ptr(RepetitionRequired), ConvertedType: ptr(ConvertedInt64), LogicalType: IntType{BitWidth: 64, IsSigned: true}},
	{Name: "b", Type: ptr(TypeByteArray), RepetitionType: ptr(RepetitionRequired), ConvertedType: ptr(ConvertedUTF8), LogicalType: StringType{}},
	{Name: "tags", Type: ptr(TypeByteArray), RepetitionType: ptr(RepetitionRepeated), ConvertedType: ptr(ConvertedUTF8), LogicalType: StringType{}},
}

func TestColumns(t *testing.T) {
	nestedRoot, err := BuildTree(nestedSchema)
	if err != nil {
		t.Fatalf("unexpected error building tree: %v", err)
	}

	gotNestedColumns := Columns(nestedRoot)

	wantNestedColumns := []Column{
		{Path: []string{"id"}, Element: nestedSchema[1], MaxDefinitionLevel: 0, MaxRepetitionLevel: 0},
		{Path: []string{"inner", "a"}, Element: nestedSchema[3], MaxDefinitionLevel: 0, MaxRepetitionLevel: 0},
		{Path: []string{"inner", "b"}, Element: nestedSchema[4], MaxDefinitionLevel: 0, MaxRepetitionLevel: 0},
		{Path: []string{"opt_in", "a"}, Element: nestedSchema[6], MaxDefinitionLevel: 1, MaxRepetitionLevel: 0},
		{Path: []string{"opt_in", "b"}, Element: nestedSchema[7], MaxDefinitionLevel: 1, MaxRepetitionLevel: 0},
		{Path: []string{"tags"}, Element: nestedSchema[8], MaxDefinitionLevel: 1, MaxRepetitionLevel: 1},
	}

	if diff := cmp.Diff(wantNestedColumns, gotNestedColumns); diff != "" {
		t.Errorf("nested columns mismatch (-want +got):\n%s", diff)
	}

	basicRoot, err := BuildTree(basicSchema)
	if err != nil {
		t.Fatalf("unexpected error building tree: %v", err)
	}

	gotBasicColumns := Columns(basicRoot)
	wantBasicColumns := []Column{
		{Path: []string{"row_number"}, Element: basicSchema[1], MaxDefinitionLevel: 0, MaxRepetitionLevel: 0},
		{Path: []string{"even_row_number"}, Element: basicSchema[2], MaxDefinitionLevel: 1, MaxRepetitionLevel: 0},
		{Path: []string{"rand_id"}, Element: basicSchema[3], MaxDefinitionLevel: 0, MaxRepetitionLevel: 0},
		{Path: []string{"opt_rand_id"}, Element: basicSchema[4], MaxDefinitionLevel: 1, MaxRepetitionLevel: 0},
		{Path: []string{"category"}, Element: basicSchema[5], MaxDefinitionLevel: 0, MaxRepetitionLevel: 0},
		{Path: []string{"rand_float"}, Element: basicSchema[6], MaxDefinitionLevel: 0, MaxRepetitionLevel: 0},
		{Path: []string{"ts"}, Element: basicSchema[7], MaxDefinitionLevel: 0, MaxRepetitionLevel: 0},
		{Path: []string{"is_odd"}, Element: basicSchema[8], MaxDefinitionLevel: 0, MaxRepetitionLevel: 0},
	}

	if diff := cmp.Diff(wantBasicColumns, gotBasicColumns); diff != "" {
		t.Errorf("basic columns mismatch (-want +got):\n%s", diff)
	}
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
		name              string
		path              string
		wantVersion       int64
		wantNumRows       int64
		wantCreatedBy     string
		wantSchema        []SchemaElement
		wantRowGroups     int
		wantRowsPerGroup  []int
		wantChunks        int
		wantRowGroupBytes []int64
	}{
		{name: "basic", path: "testdata/basic.parquet", wantVersion: 2, wantNumRows: 100, wantCreatedBy: genCreatedBy, wantSchema: basicSchema, wantRowGroups: 1, wantRowsPerGroup: []int{100}, wantChunks: 8, wantRowGroupBytes: []int64{4843}},
		{name: "empty", path: "testdata/empty.parquet", wantVersion: 2, wantNumRows: 0, wantCreatedBy: genCreatedBy, wantSchema: basicSchema, wantRowGroups: 0},
		{name: "many_rows", path: "testdata/many_rows.parquet", wantVersion: 2, wantNumRows: 300, wantCreatedBy: genCreatedBy, wantSchema: basicSchema, wantRowGroups: 3, wantRowsPerGroup: []int{100, 100, 100}, wantChunks: 8, wantRowGroupBytes: []int64{4843, 4836, 4857}},
		{name: "single_row", path: "testdata/single_row.parquet", wantVersion: 2, wantNumRows: 1, wantCreatedBy: genCreatedBy, wantSchema: basicSchema, wantRowGroups: 1, wantRowsPerGroup: []int{1}, wantChunks: 8, wantRowGroupBytes: []int64{497}},
		{name: "nested", path: "testdata/nested.parquet", wantVersion: 2, wantNumRows: 100, wantCreatedBy: genCreatedBy, wantSchema: nestedSchema, wantRowGroups: 1, wantRowsPerGroup: []int{1}, wantChunks: 6, wantRowGroupBytes: []int64{4604}},
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

			if len(fileMetadata.RowGroups) != tt.wantRowGroups {
				t.Errorf("row_groups: want %d, got %d", tt.wantRowGroups, len(fileMetadata.RowGroups))
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
	rowGroupsField := []byte{0x19, 0x0c}
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
			input:       cat([]byte{0x15, 0x04}, schemaField, []byte{0x16, 0xc8, 0x01}, rowGroupsField, []byte{0x00}),
			wantVersion: 2,
			wantNumRows: 100,
			wantSchema:  wantSchema,
			wantErr:     false,
		},
		{
			name:       "error: missing num_rows",
			input:      cat([]byte{0x15, 0x04}, schemaField, []byte{0x00}),
			wantErr:    true,
			wantErrMsg: "num_rows",
		},
		{
			name:        "version 2, num_rows 100, created_by nil, skip unknown",
			input:       cat([]byte{0x15, 0x04}, schemaField, []byte{0x16, 0xc8, 0x01}, rowGroupsField, []byte{0x05, 0x28, 0x02}, []byte{0x00}),
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
			input:         cat([]byte{0x15, 0x04}, schemaField, []byte{0x16, 0xc8, 0x01}, rowGroupsField, []byte{0x28, 0x01, 0x78}, []byte{0x00}),
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

func TestRowGroups(t *testing.T) {
	f, err := ReadFile("testdata/many_rows.parquet")
	if err != nil {
		t.Fatalf("Readfile: %v", err)
	}
	groups := f.Metadata.RowGroups

	if len(groups) != 3 {
		t.Fatalf("row groups: want 3, got %d", len(groups))
	}
	wantBytes := []int64{4843, 4836, 4857}
	for i, g := range groups {
		if g.NumRows != 100 {
			t.Errorf("group %d num_rows: want 100, got %d", i, g.NumRows)
		}
		if g.TotalByteSize != wantBytes[i] {
			t.Errorf("group %d total_byte_size: want %d, got %d", i, wantBytes[i], g.TotalByteSize)
		}
		if len(g.Columns) != 8 {
			t.Errorf("group %d columns: want 8, got %d", i, len(g.Columns))
		}
	}

	// --- one chunk in full: row_number, group 0 ---
	wantChunk := ColumnChunk{
		FilePath:   nil,
		FileOffset: 0,
		Metadata: &ColumnMetadata{
			Type:                  TypeInt64,
			Encodings:             []Encoding{EncodingPlain},
			PathInSchema:          []string{"row_number"},
			Codec:                 CodecUncompressed,
			NumValues:             100,
			TotalUncompressedSize: 876,
			TotalCompressedSize:   876,
			DataPageOffset:        4,
			DictionaryPageOffset:  nil,
			Statistics: &Statistics{
				NullCount:     ptr(int64(0)),
				DistinctCount: nil,
				MinValue:      []byte{1, 0, 0, 0, 0, 0, 0, 0},
				MaxValue:      []byte{100, 0, 0, 0, 0, 0, 0, 0},
			},
		},
	}

	if diff := cmp.Diff(wantChunk, groups[0].Columns[0]); diff != "" {
		t.Errorf("group 0 chunk 0 mismatch (-want +got):\n%s", diff)
	}

	// --- min/max across groups: non-overlapping ranges ---
	wantRanges := [][2]int64{{1, 100}, {101, 200}, {201, 300}}
	for i, g := range groups {
		s := g.Columns[0].Metadata.Statistics
		gotMin := int64(binary.LittleEndian.Uint64(s.MinValue))
		gotMax := int64(binary.LittleEndian.Uint64(s.MaxValue))
		if gotMin != wantRanges[i][0] || gotMax != wantRanges[i][1] {
			t.Errorf("group %d range: want %v, got [%d %d]", i, wantRanges[i], gotMin, gotMax)
		}
	}
}
