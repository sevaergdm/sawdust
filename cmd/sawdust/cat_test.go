package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sevaergdm/sawdust"
)

const fixtures = "../../testdata/"

func ptr[T any](v T) *T { return &v }

func TestNormalizeValues(t *testing.T) {
	tests := []struct {
		name    string
		input   sawdust.ColumnValues
		want    []any
		wantErr bool
	}{
		{
			name:  "int64 values",
			input: sawdust.Int64Values{ptr(int64(1)), ptr(int64(2))},
			want:  []any{int64(1), int64(2)},
		},
		{
			name:  "byte array becomes hex, not base64",
			input: sawdust.ByteArrayValues{ptr([]byte{0x00, 0x1b, 0x01, 0xff}), nil},
			want:  []any{"001b01ff", nil},
		},
		{
			name: "list groups by offsets",
			input: sawdust.ListValues{
				Elements: sawdust.StringValues{ptr("a"), ptr("b0"), ptr("b1"), ptr("d")},
				Offsets:  []int{0, 1, 3, 3, 4},
			},
			want: []any{[]any{"a"}, []any{"b0", "b1"}, []any{}, []any{"d"}},
		},
		{
			name:  "nulls",
			input: sawdust.Int64Values{ptr(int64(1)), nil, ptr(int64(3))},
			want:  []any{int64(1), nil, int64(3)},
		},
		{
			name:    "error: bad columntype",
			input:   sawdust.ListValues{Elements: nil, Offsets: []int{0, 1}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeValues(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr: %v, got err: %v", tt.wantErr, err)
			}

			if tt.wantErr {
				return
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):%s\n", diff)
			}
		})
	}
}

func TestWriteRows(t *testing.T) {
	var buf bytes.Buffer

	f, err := sawdust.OpenFile(fixtures + "nested.parquet")
	if err != nil {
		t.Fatalf("unexpected error opening file: %v", err)
	}
	defer func() { _ = f.Close() }()

	if err := writeRows(f, &buf); err != nil {
		t.Fatalf("writeRows: %v", err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 100 {
		t.Fatalf("want 100 lines, but got %d", len(lines))
	}

	want := []string{
		`{"id":1,"inner.a":10,"inner.b":"in-0001","opt_in.a":null,"opt_in.b":null,"tags":["tag-1-0"]}`,
		`{"id":2,"inner.a":20,"inner.b":"in-0002","opt_in.a":200,"opt_in.b":"opt-0002","tags":["tag-2-0","tag-2-1"]}`,
		`{"id":3,"inner.a":30,"inner.b":"in-0003","opt_in.a":null,"opt_in.b":null,"tags":[]}`,
	}

	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d:\n want %s\n  got %s", i, w, lines[i])
		}
	}
}
