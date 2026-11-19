//go:build unix

package engine

import "golang.org/x/sys/unix"

func shut(fd int) {
	unix.Close(fd)
}
