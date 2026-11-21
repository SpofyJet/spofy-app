package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/SpofyJet/spofy-app/core/internal/pack"
)

var (
	shellExecuteEx           = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")
	allowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
)

type executeInfo struct {
	size       uint32
	mask       uint32
	hwnd       uintptr
	verb       *uint16
	file       *uint16
	parameters *uint16
	directory  *uint16
	show       int32
	instApp    uintptr
	idList     uintptr
	class      *uint16
	keyClass   uintptr
	hotKey     uint32
	icon       uintptr
	process    windows.Handle
}

func (s *setup) serving(version string) bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, entryKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	have, _, err := k.GetStringValue("DisplayVersion")
	k.Close()
	if err != nil {
		return false
	}
	order, ok := pack.Compare(have, version)
	if !ok || order < 0 || !exists(filepath.Join(s.home, window)) || !exists(filepath.Join(s.home, core)) {
		return false
	}
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false
	}
	defer windows.CloseServiceHandle(m)
	name, _ := windows.UTF16PtrFromString(serviceName)
	sv, err := windows.OpenService(m, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return false
	}
	defer windows.CloseServiceHandle(sv)
	var st windows.SERVICE_STATUS
	if windows.QueryServiceStatus(sv, &st) != nil || st.CurrentState != windows.SERVICE_RUNNING {
		return false
	}
	s.printf("%s is installed and running, the setup of %s opens it", have, version)
	return true
}

func (s *setup) show(elevated bool) int {
	if s.quiet {
		return codeOK
	}
	path := filepath.Join(s.home, window)
	cmd := exec.Command(path)
	if root, err := windows.GetSystemWindowsDirectory(); elevated && err == nil {
		cmd = exec.Command(filepath.Join(root, "explorer.exe"), path)
	}
	cmd.Dir = s.home
	allowSetForegroundWindow.Call(0xFFFFFFFF)
	if err := cmd.Start(); err != nil {
		return s.fail(codeFailed, says().failed, err)
	}
	cmd.Process.Release()
	return codeOK
}

func (s *setup) elevate(self string, args []string, raised bool) int {
	if raised {
		return s.fail(codeFailed, says().admin, errors.New("not elevated after the request"))
	}
	line := []string{"-raised"}
	for _, a := range args {
		line = append(line, windows.EscapeArg(a))
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(self)
	params, _ := windows.UTF16PtrFromString(strings.Join(line, " "))
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(self))
	e := executeInfo{mask: 0x40 | 0x100, verb: verb, file: file, parameters: params, directory: dir, show: windows.SW_SHOWNORMAL}
	e.size = uint32(unsafe.Sizeof(e))
	if r, _, err := shellExecuteEx.Call(uintptr(unsafe.Pointer(&e))); r == 0 {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return codeCancelled
		}
		return s.fail(codeFailed, says().admin, err)
	}
	defer windows.CloseHandle(e.process)
	windows.WaitForSingleObject(e.process, windows.INFINITE)
	var code uint32
	if windows.GetExitCodeProcess(e.process, &code) != nil {
		return codeFailed
	}
	return int(code)
}
