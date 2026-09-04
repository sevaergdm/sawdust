package sawdust

func genAlt(n int) []int64 {
	out := make([]int64, 0, n)
	for i := range n {
		if i%2 == 0 {
			out = append(out, 0)
		} else {
			out = append(out, 1)
		}
	}
	return out
}

func seqOffsets(n int) []int {
	out := make([]int, 0, n+1)
	for i := range n + 1 {
		out = append(out, i)
	}
	return out
}
