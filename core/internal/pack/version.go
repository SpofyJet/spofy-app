package pack

import (
	"slices"
	"strconv"
	"strings"
)

func Compare(a, b string) (int, bool) {
	x, ok := numbers(a)
	y, fine := numbers(b)
	if !ok || !fine {
		return 0, false
	}
	for len(x) < len(y) {
		x = append(x, 0)
	}
	for len(y) < len(x) {
		y = append(y, 0)
	}
	return slices.Compare(x, y), true
}

func numbers(v string) ([]int, bool) {
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p != strconv.Itoa(n) {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}
