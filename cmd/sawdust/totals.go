package main

import (
	"fmt"
	"math"
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
	files           int
	skipped         int
	numRows         int64
	numRowGroups    int
	minRowsPerGroup int64
	maxRowsPerGroup int64
	fileBytes       int64
	byColumn        map[string]columnTotalBytes
}

func newTotals() totals {
	return totals{minRowsPerGroup: math.MaxInt64, byColumn: make(map[string]columnTotalBytes)}
}

func (t *totals) add(f sawdust.File) {
	t.files++
	t.numRows += f.Metadata.NumRows
	t.numRowGroups += len(f.Metadata.RowGroups)
	t.fileBytes += f.Size

	for _, g := range f.Metadata.RowGroups {
		if g.NumRows < t.minRowsPerGroup {
			t.minRowsPerGroup = g.NumRows
		}

		if g.NumRows > t.maxRowsPerGroup {
			t.maxRowsPerGroup = g.NumRows
		}

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
