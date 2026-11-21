package main

import (
	"encoding/binary"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	comctl32          = windows.NewLazySystemDLL("comctl32.dll")
	user32            = windows.NewLazySystemDLL("user32.dll")
	kernel32          = windows.NewLazySystemDLL("kernel32.dll")
	taskDialog        = comctl32.NewProc("TaskDialogIndirect")
	sendMessage       = user32.NewProc("SendMessageW")
	messageBox        = user32.NewProc("MessageBoxW")
	defaultUILanguage = kernel32.NewProc("GetUserDefaultUILanguage")
)

const (
	tdfAllowCancel   = 0x0008
	tdfProgress      = 0x0200
	tdfTimer         = 0x0800
	tdfSizeToContent = 0x01000000
	tdcbOK           = 0x01
	tdcbYes          = 0x02
	tdcbNo           = 0x04
	tdcbCancel       = 0x08
	iconWarning      = 0xFFFF
	iconError        = 0xFFFE
	iconInfo         = 0xFFFD
	idCancel         = 2
	idYes            = 6
	tdnCreated       = 0
	tdnButton        = 2
	tdnTimer         = 4
	tdmClick         = 0x0400 + 102
	tdmRange         = 0x0400 + 105
	tdmPos           = 0x0400 + 106
	tdmEnable        = 0x0400 + 111
	configSize       = 160
	sFalse           = 1
)

type dialog struct {
	flags, buttons uint32
	icon           uintptr
	title, main    string
	text           string
	callback       uintptr
}

func (d dialog) show() (int, bool) {
	if taskDialog.Find() != nil {
		return d.fallback()
	}
	title, _ := windows.UTF16PtrFromString(d.title)
	main, _ := windows.UTF16PtrFromString(d.main)
	text, _ := windows.UTF16PtrFromString(d.text)
	var cfg [configSize]byte
	le := binary.LittleEndian
	le.PutUint32(cfg[0:], configSize)
	le.PutUint32(cfg[20:], d.flags|tdfSizeToContent)
	le.PutUint32(cfg[24:], d.buttons)
	le.PutUint64(cfg[28:], uint64(uintptr(unsafe.Pointer(title))))
	le.PutUint64(cfg[36:], uint64(d.icon))
	le.PutUint64(cfg[44:], uint64(uintptr(unsafe.Pointer(main))))
	le.PutUint64(cfg[52:], uint64(uintptr(unsafe.Pointer(text))))
	le.PutUint64(cfg[140:], uint64(d.callback))
	var pressed int32
	r, _, _ := taskDialog.Call(uintptr(unsafe.Pointer(&cfg[0])), uintptr(unsafe.Pointer(&pressed)), 0, 0)
	runtime.KeepAlive(title)
	runtime.KeepAlive(main)
	runtime.KeepAlive(text)
	if r != 0 {
		return d.fallback()
	}
	return int(pressed), true
}

func (d dialog) fallback() (int, bool) {
	if d.callback != 0 {
		return 0, false
	}
	style := uintptr(0x40)
	switch {
	case d.icon == iconError:
		style = 0x10
	case d.icon == iconWarning:
		style = 0x30
	}
	if d.buttons&tdcbYes != 0 {
		style |= 0x04
	}
	title, _ := windows.UTF16PtrFromString(d.title)
	text, _ := windows.UTF16PtrFromString(d.main + "\n\n" + d.text)
	r, _, _ := messageBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), style|0x10000)
	return int(r), true
}

func tell(main, text string, bad bool) {
	icon := uintptr(iconInfo)
	if bad {
		icon = iconError
	}
	dialog{buttons: tdcbOK, icon: icon, title: "Spofy", main: main, text: text}.show()
}

func confirm(main, text string) bool {
	pressed, ok := dialog{buttons: tdcbYes | tdcbNo, icon: iconWarning, title: "Spofy", main: main, text: text}.show()
	return ok && pressed == idYes
}

type progress struct {
	permille    atomic.Int64
	finished    atomic.Bool
	cancellable atomic.Bool
	cancelled   atomic.Bool
	closing     bool
}

var (
	current  atomic.Pointer[progress]
	callback = sync.OnceValue(func() uintptr {
		return windows.NewCallback(func(hwnd windows.HWND, msg uint32, wparam, lparam, _ uintptr) uintptr {
			p := current.Load()
			if p == nil {
				return 0
			}
			switch msg {
			case tdnCreated:
				sendMessage.Call(uintptr(hwnd), tdmRange, 0, 1000<<16)
			case tdnTimer:
				sendMessage.Call(uintptr(hwnd), tdmPos, uintptr(p.permille.Load()), 0)
				enable := uintptr(0)
				if p.cancellable.Load() && !p.cancelled.Load() {
					enable = 1
				}
				sendMessage.Call(uintptr(hwnd), tdmEnable, idCancel, enable)
				if p.finished.Load() && !p.closing {
					p.closing = true
					sendMessage.Call(uintptr(hwnd), tdmClick, idCancel, 0)
				}
			case tdnButton:
				if p.closing {
					return 0
				}
				if wparam == idCancel && p.cancellable.Load() {
					p.cancelled.Store(true)
				}
				return sFalse
			}
			return 0
		})
	})
)

func (p *progress) run(quiet bool, main string, work func()) {
	if quiet {
		work()
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer p.finished.Store(true)
		work()
	}()
	current.Store(p)
	_, shown := dialog{flags: tdfProgress | tdfTimer | tdfAllowCancel, buttons: tdcbCancel, title: "Spofy", main: main, callback: callback()}.show()
	current.Store(nil)
	if !shown {
		<-done
	}
}

func russian() bool {
	lang, _, _ := defaultUILanguage.Call()
	switch lang & 0x3ff {
	case 0x19, 0x22, 0x23, 0x3f:
		return true
	}
	return false
}
