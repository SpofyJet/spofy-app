package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/SpofyJet/spofy-app/core/internal/desk"
)

func greet(root string, logs io.Writer) {
	runs := desk.Invited(root, time.Now())
	if len(runs) == 0 {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	for _, r := range runs {
		if err := startIn(r.Session, filepath.Join(filepath.Dir(exe), r.Program)); err != nil {
			fmt.Fprintf(logs, "cannot start %s in session %d: %v\n", r.Program, r.Session, err)
		}
	}
}

func startIn(session uint32, path string) error {
	h, err := spawnIn(session, path)
	if err != nil {
		return err
	}
	windows.CloseHandle(h)
	return nil
}

func spawnIn(session uint32, path string, args ...string) (windows.Handle, error) {
	var token windows.Token
	if err := windows.WTSQueryUserToken(session, &token); err != nil {
		return 0, err
	}
	defer token.Close()
	var env *uint16
	if err := windows.CreateEnvironmentBlock(&env, token, false); err != nil {
		return 0, err
	}
	defer windows.DestroyEnvironmentBlock(env)
	desktop, err := windows.UTF16PtrFromString(`winsta0\default`)
	if err != nil {
		return 0, err
	}
	si := windows.StartupInfo{Desktop: desktop}
	si.Cb = uint32(unsafe.Sizeof(si))
	app, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	line, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{path}, args...)))
	if err != nil {
		return 0, err
	}
	dir, err := windows.UTF16PtrFromString(filepath.Dir(path))
	if err != nil {
		return 0, err
	}
	var pi windows.ProcessInformation
	if err := windows.CreateProcessAsUser(token, app, line, nil, nil, false, windows.CREATE_UNICODE_ENVIRONMENT, env, dir, &si, &pi); err != nil {
		return 0, err
	}
	windows.CloseHandle(pi.Thread)
	return pi.Process, nil
}
