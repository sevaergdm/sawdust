package main

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/sevaergdm/sawdust"
)

type columnTotalBytes struct {
	name              string
	compressedBytes   int64
	uncompressedBytes int64
}

type totals struct {
	files        int
	skipped      int
	numRows      int64
	rowsPerGroup []int64
	fileBytes    int64
	byColumn     map[string]columnTotalBytes
}

func newTotals() totals {
	return totals{byColumn: make(map[string]columnTotalBytes)}
}

func (t *totals) add(f *sawdust.File) {
	t.files++
	t.numRows += f.Metadata.NumRows
	t.fileBytes += f.Size

	for _, g := range f.Metadata.RowGroups {
		t.rowsPerGroup = append(t.rowsPerGroup, g.NumRows)

		for _, c := range g.Columns {
			colName := strings.Join(c.Metadata.PathInSchema, ".")
			tmp := t.byColumn[colName]
			tmp.compressedBytes += c.Metadata.TotalCompressedSize
			tmp.uncompressedBytes += c.Metadata.TotalUncompressedSize
			t.byColumn[colName] = tmp
		}
	}
}

func (t *totals) ranking() []columnTotalBytes {
	var sortedColumns []columnTotalBytes
	for k, v := range t.byColumn {
		sortedColumns = append(sortedColumns, columnTotalBytes{name: k, compressedBytes: v.compressedBytes, uncompressedBytes: v.uncompressedBytes})
	}

	sort.SliceStable(sortedColumns, func(i, j int) bool {
		return sortedColumns[i].compressedBytes > sortedColumns[j].compressedBytes
	})
	return sortedColumns
}

func orDash(p *int64) string {
	if p == nil {
		return "-"
	}
	return strconv.FormatInt(*p, 10)
}

func compressedRatio(cBytes, uBytes int64) string {
	if cBytes == 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f", float64(uBytes)/float64(cBytes))
}

func (t *totals) rowsPerGroupSummary() string {
	if len(t.rowsPerGroup) == 0 {
		return ""
	}
	return fmt.Sprintf("rows/group: min %d mean %d max %d", slices.Min(t.rowsPerGroup), t.numRows/int64(len(t.rowsPerGroup)), slices.Max(t.rowsPerGroup))
}

// rowsPerGroupStat assumes only one file and is only useful for the stat command
func (t *totals) rowsPerGroupStat() string {
	var rowGroupString strings.Builder
	switch len(t.rowsPerGroup) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("(%d)", t.rowsPerGroup[0])
	default:
		for i, group := range t.rowsPerGroup {
			numRows := fmt.Sprintf("%d", group)
			if i == 0 {
				rowGroupString.WriteString("(" + numRows + ", ")
			} else if i == len(t.rowsPerGroup)-1 {
				rowGroupString.WriteString(numRows + ")")
			} else {
				rowGroupString.WriteString(numRows + ", ")
			}
		}
	}
	return rowGroupString.String()
}
