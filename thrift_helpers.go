package sawdust

import "fmt"

func ptr[T any](v T) *T { return &v }

type requiredField struct {
	id   int64
	name string
}

func checkRequired(seen map[int64]bool, fields []requiredField) error {
	for _, f := range fields {
		if !seen[f.id] {
			return fmt.Errorf("missing required field %d (%s)", f.id, f.name)
		}
	}
	return nil
}
