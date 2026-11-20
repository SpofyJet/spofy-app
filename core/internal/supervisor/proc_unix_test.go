//go:build unix

package supervisor

import (
	"errors"
	"syscall"
)

func kill(pid int) {
	syscall.Kill(pid, syscall.SIGKILL)
}

func gone(pid int) bool {
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}
