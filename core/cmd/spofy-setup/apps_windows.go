package main

import (
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/SpofyJet/spofy-app/core/internal/desk"
)

const serviceName = "Spofy"

var (
	enumWindows              = user32.NewProc("EnumWindows")
	getWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	postMessage              = user32.NewProc("PostMessageW")
)

type app struct {
	process windows.Handle
	pid     uint32
	session uint32
	name    string
}

func running(home string) []app {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var out []app
	e := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if e.ProcessID == windows.GetCurrentProcessId() {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, e.ProcessID)
		if err != nil {
			continue
		}
		path, err := image(h)
		if err != nil || !strings.EqualFold(filepath.Dir(path), home) {
			windows.CloseHandle(h)
			continue
		}
		a := app{process: h, pid: e.ProcessID, name: strings.ToLower(filepath.Base(path))}
		windows.ProcessIdToSessionId(e.ProcessID, &a.session)
		out = append(out, a)
	}
	return out
}

func image(h windows.Handle) (string, error) {
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:n]), nil
}

func (s *setup) closeApps(grace time.Duration, names ...string) []desk.Run {
	apps := slices.DeleteFunc(running(s.home), func(a app) bool {
		if !slices.Contains(names, a.name) {
			windows.CloseHandle(a.process)
			return true
		}
		return false
	})
	defer func() {
		for _, a := range apps {
			windows.CloseHandle(a.process)
		}
	}()
	var ran []desk.Run
	for _, a := range apps {
		if !slices.Contains(ran, desk.Run{Session: a.session, Program: a.name}) {
			ran = append(ran, desk.Run{Session: a.session, Program: a.name})
		}
		if a.session == s.session {
			closeWindows(a.pid)
		}
	}
	deadline := time.Now().Add(grace)
	for _, a := range apps {
		wait := max(time.Until(deadline), 0)
		if ev, _ := windows.WaitForSingleObject(a.process, uint32(wait.Milliseconds())); ev != windows.WAIT_OBJECT_0 {
			s.printf("%s (%d) did not close in session %d, ending it", a.name, a.pid, a.session)
			windows.TerminateProcess(a.process, 1)
			windows.WaitForSingleObject(a.process, 5000)
		}
	}
	s.printf("closed %d programs: %+v", len(apps), ran)
	return ran
}

var closer = windows.NewCallback(func(hwnd, pid uintptr) uintptr {
	var owner uint32
	getWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&owner)))
	if uintptr(owner) == pid {
		postMessage.Call(hwnd, 0x0010, 0, 0)
	}
	return 1
})

func closeWindows(pid uint32) {
	enumWindows.Call(closer, uintptr(pid))
}

func stopService() (bool, error) {
	m, err := mgr.Connect()
	if err != nil {
		return false, err
	}
	defer m.Disconnect()
	sv, err := m.OpenService(serviceName)
	if err != nil {
		return false, nil
	}
	defer sv.Close()
	q, err := sv.Query()
	if err != nil {
		return true, err
	}
	if q.State == svc.Stopped {
		return true, nil
	}
	if q.State != svc.StopPending {
		if _, err := sv.Control(svc.Stop); err != nil && q.State != svc.StartPending {
			return true, err
		}
	}
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if q, err = sv.Query(); err != nil || q.State == svc.Stopped {
			return true, err
		}
	}
	return true, windows.ERROR_SERVICE_REQUEST_TIMEOUT
}
