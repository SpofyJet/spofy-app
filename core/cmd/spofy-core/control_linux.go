package main

import (
	"os"

	"golang.org/x/sys/unix"
)

func controlChannel() *os.File {
	fd, err := unix.Dup(1)
	if err != nil {
		return os.Stdout
	}
	unix.CloseOnExec(fd)
	if err := unix.Dup3(2, 1, 0); err != nil {
		return os.Stdout
	}
	return os.NewFile(uintptr(fd), "control")
}
