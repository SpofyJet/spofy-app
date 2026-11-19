//go:build !linux

package main

import "os"

func controlChannel() *os.File {
	ctl := os.Stdout
	os.Stdout = os.Stderr
	return ctl
}
