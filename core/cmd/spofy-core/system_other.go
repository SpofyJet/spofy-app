//go:build !windows

package main

func system(string, []string) (int, bool) {
	return 0, false
}
