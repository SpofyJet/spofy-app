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
	getShellWindow           = user32.NewProc("GetShellWindow")
	createProcessWithToken   = windows.NewLazySystemDLL("advapi32.dll").NewProc("CreateProcessWithTokenW")
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
	allowSetForegroundWindow.Call(0xFFFFFFFF)
	if elevated {
		err := asShell(path, s.home)
		if err == nil {
			return codeOK
		}
		s.printf("the window opens with the rights of the setup: %v", err)
	}
	cmd := exec.Command(path)
	cmd.Dir = s.home
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

func asShell(path, dir string) error {
	hwnd, _, _ := getShellWindow.Call()
	if hwnd == 0 {
		return errors.New("no shell on this desktop")
	}
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(windows.HWND(hwnd), &pid); err != nil {
		return err
	}
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(p)
	var shell windows.Token
	if err := windows.OpenProcessToken(p, windows.TOKEN_DUPLICATE, &shell); err != nil {
		return err
	}
	defer shell.Close()
	var t windows.Token
	if err := windows.DuplicateTokenEx(shell, windows.TOKEN_QUERY|windows.TOKEN_DUPLICATE|windows.TOKEN_ASSIGN_PRIMARY, nil, windows.SecurityImpersonation, windows.TokenPrimary, &t); err != nil {
		return err
	}
	defer t.Close()
	app, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	line, err := windows.UTF16PtrFromString(windows.EscapeArg(path))
	if err != nil {
		return err
	}
	wd, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	si := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var pi windows.ProcessInformation
	if r, _, err := createProcessWithToken.Call(uintptr(t), 0, uintptr(unsafe.Pointer(app)), uintptr(unsafe.Pointer(line)), 0, 0,
		uintptr(unsafe.Pointer(wd)), uintptr(unsafe.Pointer(&si)), uintptr(unsafe.Pointer(&pi))); r == 0 {
		return err
	}
	windows.CloseHandle(pi.Thread)
	windows.CloseHandle(pi.Process)
	return nil
}
