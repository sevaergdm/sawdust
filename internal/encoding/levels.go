package encoding

import "fmt"

// applyLevels expands values into one slot per row. The result has one entry per level:
// a pointer to the next unused value where the level equals maxDefLevel, and nil where it does not,
// since a null consumes no value.
//
// The returned pointers alias values rather than copying it. Mutating values after the call changes
// the result, and retaining any non-nil pointer keeps the whole values backing array alive.
func ApplyLevels[T any](repLevels, defLevels []int64, values []T, maxDefLevel, maxRepLevel int64, numRows int) ([]*T, []int, error) {
	var elements []*T
	offsets := make([]int, 0, numRows+1)
	cursor := 0

	if maxRepLevel > 0 && len(repLevels) != len(defLevels) {
		return nil, nil, fmt.Errorf("repetition levels (%d) and definition levels (%d) should match", len(repLevels), len(defLevels))
	}

	if maxRepLevel > 1 {
		return nil, nil, fmt.Errorf("nesting deeper than one level is not supported (maxRepLevel %d)", maxRepLevel)
	}

	if maxRepLevel > 0 && maxDefLevel > maxRepLevel {
		return nil, nil, fmt.Errorf("repeated column with nullable elements is not supported (maxDefLevel %d, maxRepLevel %d)", maxDefLevel, maxRepLevel)
	}

	for i := range defLevels {
		if maxRepLevel == 0 || repLevels[i] == 0 {
			offsets = append(offsets, len(elements))
		}
		if defLevels[i] == maxDefLevel {
			if cursor >= len(values) {
				return nil, nil, fmt.Errorf("levels need at least %d values, but only %d were stored", cursor+1, len(values))
			}
			elements = append(elements, &values[cursor])
			cursor++
		} else if maxRepLevel == 0 {
			elements = append(elements, nil)
		}
	}
	offsets = append(offsets, len(elements))

	if len(offsets)-1 != numRows {
		return nil, nil, fmt.Errorf("levels produced %d rows, header says %d", len(offsets)-1, numRows)
	}

	if cursor != len(values) {
		return nil, nil, fmt.Errorf("levels said %d values were present, but only %d were stored", len(values), cursor)
	}
	return elements, offsets, nil
}
