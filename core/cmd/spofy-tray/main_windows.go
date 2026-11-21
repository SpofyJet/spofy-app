package main

import (
	"context"
	"embed"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/SpofyJet/spofy-app/core/internal/tray"
)

const (
	pipePath    = `\\.\pipe\spofy`
	className   = "SPOFY_TRAY"
	windowClass = "SPOFY_WINDOW"
	iconID      = 1

	wmNull            = 0x0000
	wmDestroy         = 0x0002
	wmClose           = 0x0010
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
	wmContextMenu     = 0x007B
	wmTimer           = 0x0113
	wmUser            = 0x0400
	wmTray            = 0x8001
	wmWake            = 0x8002

	ninSelect           = wmUser
	ninKeySelect        = wmUser + 1
	ninBalloonUserClick = wmUser + 5

	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nimSetVersion = 4

	nifMessage = 0x01
	nifIcon    = 0x02
	nifTip     = 0x04
	nifInfo    = 0x10
	nifShowTip = 0x80

	niifUser             = 0x04
	niifLargeIcon        = 0x20
	niifRespectQuietTime = 0x80

	mfString    = 0x000
	mfGrayed    = 0x001
	mfSeparator = 0x800

	tpmRightButton = 0x002
	tpmRightAlign  = 0x008
	tpmBottomAlign = 0x020
	tpmNoNotify    = 0x080
	tpmReturnCmd   = 0x100

	smCxIcon            = 11
	smMenuDropAlignment = 40
	smCxSmIcon          = 49

	cmdToggle = 1
	cmdOpen   = 2
	cmdQuit   = 3

	timerAdd    = 1
	timerStall  = 2
	timerQuit   = 3
	timerRemind = 4

	addEvery    = 2000
	stallWait   = 20000
	quitWait    = 5000
	remindEvery = 30 * 60 * 1000
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	registerClassEx             = user32.NewProc("RegisterClassExW")
	createWindowEx              = user32.NewProc("CreateWindowExW")
	defWindowProc               = user32.NewProc("DefWindowProcW")
	destroyWindow               = user32.NewProc("DestroyWindow")
	getMessage                  = user32.NewProc("GetMessageW")
	translateMessage            = user32.NewProc("TranslateMessage")
	dispatchMessage             = user32.NewProc("DispatchMessageW")
	postMessage                 = user32.NewProc("PostMessageW")
	postQuitMessage             = user32.NewProc("PostQuitMessage")
	registerWindowMessage       = user32.NewProc("RegisterWindowMessageW")
	changeWindowMessageFilterEx = user32.NewProc("ChangeWindowMessageFilterEx")
	createPopupMenu             = user32.NewProc("CreatePopupMenu")
	appendMenu                  = user32.NewProc("AppendMenuW")
	setMenuDefaultItem          = user32.NewProc("SetMenuDefaultItem")
	trackPopupMenuEx            = user32.NewProc("TrackPopupMenuEx")
	destroyMenu                 = user32.NewProc("DestroyMenu")
	setForegroundWindow         = user32.NewProc("SetForegroundWindow")
	allowSetForegroundWindow    = user32.NewProc("AllowSetForegroundWindow")
	setTimer                    = user32.NewProc("SetTimer")
	killTimer                   = user32.NewProc("KillTimer")
	createIconFromResourceEx    = user32.NewProc("CreateIconFromResourceEx")
	destroyIcon                 = user32.NewProc("DestroyIcon")
	getSystemMetrics            = user32.NewProc("GetSystemMetrics")
	setProcessDpiAwarenessCtx   = user32.NewProc("SetProcessDpiAwarenessContext")
	findWindow                  = user32.NewProc("FindWindowW")
	isWindowVisible             = user32.NewProc("IsWindowVisible")
	isIconic                    = user32.NewProc("IsIconic")
	shellNotifyIcon             = shell32.NewProc("Shell_NotifyIconW")
	registerApplicationRestart  = kernel32.NewProc("RegisterApplicationRestart")
)

//go:embed icons/*.ico
var art embed.FS

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type msg struct {
	Hwnd    windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	X, Y    int32
	Private uint32
}

type notifyIconData struct {
	Size            uint32
	Wnd             windows.HWND
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            windows.Handle
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GuidItem        windows.GUID
	BalloonIcon     windows.Handle
}

func init() {
	runtime.LockOSThread()
}

func main() {
	os.Exit(run())
}

func run() int {
	if setProcessDpiAwarenessCtx.Find() == nil {
		setProcessDpiAwarenessCtx.Call(^uintptr(3))
	}
	name, _ := windows.UTF16PtrFromString(`Local\app.spofy.tray`)
	single, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		windows.CloseHandle(single)
		return 0
	}
	if err != nil {
		return 1
	}
	defer windows.CloseHandle(single)
	registerApplicationRestart.Call(0, 1|2|8)
	s := &shell{}
	s.model.System, _ = windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err := s.create(); err != nil {
		return 1
	}
	setTimer.Call(uintptr(s.hwnd), timerRemind, remindEvery, 0)
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan struct{})
	go func() {
		defer close(served)
		s.serve(ctx, pipePath)
	}()
	loop()
	cancel()
	<-served
	release(s.icons[:]...)
	release(s.large)
	return 0
}

func loop() {
	var m msg
	for {
		r, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&m)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (s *shell) create() error {
	var inst windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &inst); err != nil {
		return err
	}
	class, _ := windows.UTF16PtrFromString(className)
	wc := wndClassEx{WndProc: windows.NewCallback(s.proc), Instance: inst, ClassName: class}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := registerClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return err
	}
	title, _ := windows.UTF16PtrFromString(tray.App)
	h, _, err := createWindowEx.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 0, 0, 0, 0, uintptr(inst), 0)
	if h == 0 {
		return err
	}
	s.hwnd = windows.HWND(h)
	s.wake = func() { postMessage.Call(h, wmWake, 0, 0) }
	taskbar, _ := windows.UTF16PtrFromString("TaskbarCreated")
	r, _, _ := registerWindowMessage.Call(uintptr(unsafe.Pointer(taskbar)))
	s.taskbar = uint32(r)
	if r != 0 {
		changeWindowMessageFilterEx.Call(h, r, 1, 0)
	}
	if _, err := s.load(); err != nil {
		destroyWindow.Call(h)
		return err
	}
	s.refresh()
	return nil
}

func (s *shell) load() ([]windows.Handle, error) {
	small, large := metric(smCxSmIcon, 16), metric(smCxIcon, 32)
	names := [...]string{tray.Off: "off", tray.Busy: "busy", tray.On: "on", tray.Alert: "alert"}
	var fresh [len(names)]windows.Handle
	for kind, name := range names {
		h, err := iconFile(name, small)
		if err != nil {
			release(fresh[:kind]...)
			return nil, err
		}
		fresh[kind] = h
	}
	big, err := iconFile("on", large)
	if err != nil {
		release(fresh[:]...)
		return nil, err
	}
	old := append([]windows.Handle{s.large}, s.icons[:]...)
	s.icons, s.large = fresh, big
	return old, nil
}

func iconFile(name string, size int) (windows.Handle, error) {
	data, err := art.ReadFile("icons/" + name + ".ico")
	if err != nil {
		return 0, err
	}
	return makeIcon(data, size)
}

func release(icons ...windows.Handle) {
	for _, h := range icons {
		if h != 0 {
			destroyIcon.Call(uintptr(h))
		}
	}
}

func metric(index uintptr, fallback int) int {
	if r, _, _ := getSystemMetrics.Call(index); int32(r) > 0 {
		return int(int32(r))
	}
	return fallback
}

func (s *shell) proc(hwnd, m, w, l uintptr) uintptr {
	switch uint32(m) {
	case wmTray:
		switch uint32(l & 0xffff) {
		case ninSelect, ninKeySelect, ninBalloonUserClick:
			open()
		case wmContextMenu:
			s.menu(int32(int16(w)), int32(int16(w>>16)))
		}
		return 0
	case wmWake:
		s.refresh()
		return 0
	case wmTimer:
		switch w {
		case timerAdd:
			s.place()
		case timerStall:
			killTimer.Call(hwnd, timerStall)
			s.mu.Lock()
			n := s.model.StalledNote()
			s.mu.Unlock()
			if n != nil {
				s.notify(*n)
			}
		case timerQuit:
			destroyWindow.Call(hwnd)
		case timerRemind:
			s.reminder()
		}
		return 0
	case wmQueryEndSession:
		return 1
	case wmEndSession:
		if w != 0 {
			destroyWindow.Call(hwnd)
		}
		return 0
	case wmClose:
		destroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		if s.added {
			notifyIcon(nimDelete, s.data(0))
			s.added = false
		}
		postQuitMessage.Call(0)
		return 0
	}
	if s.taskbar != 0 && uint32(m) == s.taskbar {
		old, err := s.load()
		s.added = false
		s.place()
		if err == nil {
			release(old...)
		}
		return 0
	}
	r, _, _ := defWindowProc.Call(hwnd, m, w, l)
	return r
}

func (s *shell) refresh() {
	s.mu.Lock()
	v := s.model.View()
	linked := s.model.Linked()
	notes, answered := s.notes, s.answered
	s.notes, s.answered = nil, nil
	s.mu.Unlock()
	if v != s.view || !s.added {
		s.view = v
		s.place()
	}
	if v.Stalled != s.stalled {
		s.stalled = v.Stalled
		if v.Stalled {
			setTimer.Call(uintptr(s.hwnd), timerStall, stallWait, 0)
		} else {
			killTimer.Call(uintptr(s.hwnd), timerStall)
		}
	}
	for _, n := range notes {
		s.notify(n)
	}
	s.reminder()
	if s.quitting != 0 && (!linked || slices.Contains(answered, s.quitting)) {
		destroyWindow.Call(uintptr(s.hwnd))
	}
}

func (s *shell) data(flags uint32) *notifyIconData {
	d := &notifyIconData{Wnd: s.hwnd, ID: iconID, Flags: flags, CallbackMessage: wmTray, Icon: s.icons[s.view.Icon]}
	d.Size = uint32(unsafe.Sizeof(*d))
	put(d.Tip[:], s.view.Tip)
	return d
}

func (s *shell) place() {
	d := s.data(nifMessage | nifIcon | nifTip | nifShowTip)
	if notifyIcon(nimModify, d) {
		s.added = true
	} else if notifyIcon(nimAdd, d) {
		d.Version = 4
		notifyIcon(nimSetVersion, d)
		s.added = true
	} else {
		s.added = false
		setTimer.Call(uintptr(s.hwnd), timerAdd, addEvery, 0)
		return
	}
	killTimer.Call(uintptr(s.hwnd), timerAdd)
}

func (s *shell) reminder() {
	s.mu.Lock()
	key, n := s.model.Reminder(time.Now())
	s.mu.Unlock()
	if n == nil || !s.notify(*n) {
		return
	}
	s.mu.Lock()
	s.model.Reminded(key)
	s.mu.Unlock()
	s.set(tray.RemindedKey, key)
}

func (s *shell) notify(n tray.Note) bool {
	if !s.added || windowShown() {
		return false
	}
	d := s.data(nifInfo)
	put(d.InfoTitle[:], n.Title)
	put(d.Info[:], n.Text)
	d.InfoFlags = niifUser | niifLargeIcon | niifRespectQuietTime
	d.BalloonIcon = s.large
	return notifyIcon(nimModify, d)
}

func notifyIcon(op uintptr, d *notifyIconData) bool {
	r, _, _ := shellNotifyIcon.Call(op, uintptr(unsafe.Pointer(d)))
	return r != 0
}

func put(dst []uint16, text string) {
	u := utf16.Encode([]rune(tray.Clip(text, len(dst)-1)))
	dst[copy(dst, u)] = 0
}

func (s *shell) menu(x, y int32) {
	v := s.view
	m, _, _ := createPopupMenu.Call()
	if m == 0 {
		return
	}
	defer destroyMenu.Call(m)
	item := func(flags, id uintptr, label string) {
		text, err := windows.UTF16PtrFromString(strings.ReplaceAll(label, "&", "&&"))
		if err == nil {
			appendMenu.Call(m, flags, id, uintptr(unsafe.Pointer(text)))
		}
	}
	item(mfString|mfGrayed, 0, v.Status)
	appendMenu.Call(m, mfSeparator, 0, 0)
	toggle := uintptr(mfString)
	if v.Toggle.Grey {
		toggle |= mfGrayed
	}
	item(toggle, cmdToggle, v.Toggle.Label)
	item(mfString, cmdOpen, v.Open)
	appendMenu.Call(m, mfSeparator, 0, 0)
	item(mfString, cmdQuit, v.Quit.Label)
	setMenuDefaultItem.Call(m, cmdOpen, 0)
	align := uintptr(0)
	if metric(smMenuDropAlignment, 0) != 0 {
		align = tpmRightAlign
	}
	setForegroundWindow.Call(uintptr(s.hwnd))
	cmd, _, _ := trackPopupMenuEx.Call(m, align|tpmBottomAlign|tpmRightButton|tpmReturnCmd|tpmNoNotify, uintptr(x), uintptr(y), uintptr(s.hwnd), 0)
	postMessage.Call(uintptr(s.hwnd), wmNull, 0, 0)
	s.command(cmd, v)
}

func (s *shell) command(cmd uintptr, seen tray.View) {
	switch cmd {
	case cmdOpen:
		open()
	case cmdToggle:
		s.mu.Lock()
		now := s.model.View()
		s.mu.Unlock()
		if seen.Toggle.Grey || now.Toggle.Action != seen.Toggle.Action {
			return
		}
		switch seen.Toggle.Action {
		case tray.Open:
			open()
		case tray.Connect:
			s.ask("connect", tray.Connect)
		case tray.Disconnect:
			s.ask("disconnect", tray.Disconnect)
		}
	case cmdQuit:
		if s.quitting != 0 {
			return
		}
		if seen.Quit.Action == tray.Disconnect {
			if id := s.ask("disconnect", tray.Disconnect); id != 0 {
				s.quitting = id
				setTimer.Call(uintptr(s.hwnd), timerQuit, quitWait, 0)
				return
			}
		}
		destroyWindow.Call(uintptr(s.hwnd))
	}
}

func open() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	dir := filepath.Dir(exe)
	allowSetForegroundWindow.Call(0xFFFFFFFF)
	cmd := exec.Command(filepath.Join(dir, "spofy.exe"))
	cmd.Dir = dir
	if cmd.Start() == nil {
		cmd.Process.Release()
	}
}

func windowShown() bool {
	name, _ := windows.UTF16PtrFromString(windowClass)
	w, _, _ := findWindow.Call(uintptr(unsafe.Pointer(name)), 0)
	if w == 0 {
		return false
	}
	visible, _, _ := isWindowVisible.Call(w)
	small, _, _ := isIconic.Call(w)
	return visible != 0 && small == 0
}
