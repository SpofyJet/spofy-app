package main

import (
	"errors"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ole32            = windows.NewLazySystemDLL("ole32.dll")
	coCreateInstance = ole32.NewProc("CoCreateInstance")
	clsidShellLink   = windows.GUID{Data1: 0x00021401, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidShellLink     = windows.GUID{Data1: 0x000214F9, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidPersistFile   = windows.GUID{Data1: 0x0000010B, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
)

const (
	queryInterface      = 0
	release             = 2
	linkSetDescription  = 7
	linkSetWorkingDir   = 9
	linkSetIconLocation = 17
	linkSetPath         = 20
	persistSave         = 6
)

type object struct{ vtbl *[32]uintptr }

func (o *object) call(method int, args ...uintptr) error {
	r, _, _ := syscall.SyscallN(o.vtbl[method], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	if int32(r) < 0 {
		return syscall.Errno(r)
	}
	return nil
}

func shortcut(link, target, description string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && !errors.Is(err, syscall.Errno(1)) {
		return err
	}
	defer windows.CoUninitialize()
	var sl *object
	if r, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidShellLink)), 0, 1, uintptr(unsafe.Pointer(&iidShellLink)), uintptr(unsafe.Pointer(&sl))); int32(r) < 0 {
		return syscall.Errno(r)
	}
	defer sl.call(release)
	path, _ := windows.UTF16PtrFromString(target)
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(target))
	text, _ := windows.UTF16PtrFromString(description)
	for _, step := range []func() error{
		func() error { return sl.call(linkSetPath, uintptr(unsafe.Pointer(path))) },
		func() error { return sl.call(linkSetWorkingDir, uintptr(unsafe.Pointer(dir))) },
		func() error { return sl.call(linkSetDescription, uintptr(unsafe.Pointer(text))) },
		func() error { return sl.call(linkSetIconLocation, uintptr(unsafe.Pointer(path)), 0) },
	} {
		if err := step(); err != nil {
			return err
		}
	}
	var pf *object
	if err := sl.call(queryInterface, uintptr(unsafe.Pointer(&iidPersistFile)), uintptr(unsafe.Pointer(&pf))); err != nil {
		return err
	}
	defer pf.call(release)
	name, _ := windows.UTF16PtrFromString(link)
	err := pf.call(persistSave, uintptr(unsafe.Pointer(name)), 1)
	runtime.KeepAlive(path)
	runtime.KeepAlive(dir)
	runtime.KeepAlive(text)
	runtime.KeepAlive(name)
	return err
}
