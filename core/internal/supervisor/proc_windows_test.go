package supervisor

import (
	"os"

	"golang.org/x/sys/windows"
)

func kill(pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		p.Kill()
		p.Release()
	}
}

func gone(pid int) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return true
	}
	defer windows.CloseHandle(h)
	state, _ := windows.WaitForSingleObject(h, 0)
	return state == windows.WAIT_OBJECT_0
}
