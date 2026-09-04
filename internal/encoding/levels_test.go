package encoding

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

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
			gotColumnValues, gotOffsets, err := ApplyLevels(tt.repLevels, tt.defLevels, tt.values, tt.maxDefLevel, tt.maxRepLevel, tt.numRows)
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

	gotColumnValues, gotOffsets, err := ApplyLevels(repLevels, defLevels, values, 1, 0, 5)
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
