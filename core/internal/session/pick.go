package session

import (
	"sort"
	"time"
)

type Result struct {
	Tier  int
	Delay time.Duration
	OK    bool
}

func Order(results []Result, keep int) []int {
	var order []int
	for i, r := range results {
		if r.OK {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(a, b int) bool {
		x, y := results[order[a]], results[order[b]]
		if x.Tier != y.Tier {
			return x.Tier < y.Tier
		}
		return x.Delay < y.Delay
	})
	if len(order) < 2 || keep < 0 || keep >= len(results) || !results[keep].OK {
		return order
	}
	best, kept := results[order[0]], results[keep]
	if kept.Tier != best.Tier || kept.Delay > best.Delay+best.Delay/5+20*time.Millisecond {
		return order
	}
	sticky := []int{keep}
	for _, i := range order {
		if i != keep {
			sticky = append(sticky, i)
		}
	}
	return sticky
}
