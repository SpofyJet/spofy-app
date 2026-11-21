package main

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	helperWait      = 10000
	wtsSessionLogon = 5
)

type desktops struct {
	helper string
	logs   io.Writer
	mu     sync.Mutex
	want   string
}

func (d *desktops) Set(address string) error {
	return d.everywhere(address)
}

func (d *desktops) Clear() error {
	return d.everywhere("")
}

func (d *desktops) again() {
	d.mu.Lock()
	want := d.want
	d.mu.Unlock()
	if err := d.everywhere(want); err != nil {
		fmt.Fprintln(d.logs, "system proxy at logon:", err)
	}
}

func (d *desktops) everywhere(address string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.want = address
	list, err := people()
	if err != nil {
		return err
	}
	var errs []error
	for _, s := range list {
		errs = append(errs, d.in(s, address))
	}
	return errors.Join(errs...)
}

func (d *desktops) in(session uint32, address string) error {
	args := []string{"proxy", "off"}
	if address != "" {
		args = []string{"proxy", "on", address}
	}
	h, err := spawnIn(session, d.helper, args...)
	if err != nil {
		return fmt.Errorf("session %d: %w", session, err)
	}
	defer windows.CloseHandle(h)
	if ev, _ := windows.WaitForSingleObject(h, helperWait); ev != windows.WAIT_OBJECT_0 {
		windows.TerminateProcess(h, 1)
		return fmt.Errorf("session %d: the helper did not finish in %d ms", session, helperWait)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return fmt.Errorf("session %d: %w", session, err)
	}
	if code != 0 {
		return fmt.Errorf("session %d: the helper ended with %d", session, code)
	}
	return nil
}

func people() ([]uint32, error) {
	var list *windows.WTS_SESSION_INFO
	var n uint32
	if err := windows.WTSEnumerateSessions(0, 0, 1, &list, &n); err != nil {
		return nil, err
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(list)))
	var out []uint32
	for _, s := range unsafe.Slice(list, n) {
		var t windows.Token
		if s.SessionID != 0 && windows.WTSQueryUserToken(s.SessionID, &t) == nil {
			t.Close()
			out = append(out, s.SessionID)
		}
	}
	return out, nil
}
