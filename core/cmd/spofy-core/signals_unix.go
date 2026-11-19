//go:build !windows

package main

import (
	"os"
	"syscall"
)

var statSignals = []os.Signal{syscall.SIGUSR1}
