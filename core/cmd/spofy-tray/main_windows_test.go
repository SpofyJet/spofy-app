package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/ipc/namedpipe"

	"github.com/SpofyJet/spofy-app/core/internal/ipc"
	"github.com/SpofyJet/spofy-app/core/internal/tray"
)

var pipes atomic.Int64

func listen(t *testing.T, access string) (string, net.Listener) {
	t.Helper()
	path := fmt.Sprintf(`\\.\pipe\spofy-tray-test-%d-%d`, os.Getpid(), pipes.Add(1))
	sd, err := windows.SecurityDescriptorFromString(access)
	if err != nil {
		t.Fatal(err)
	}
	l, err := (&namedpipe.ListenConfig{SecurityDescriptor: sd, InputBufferSize: 64 << 10, OutputBufferSize: 64 << 10}).Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return path, l
}

func accept(t *testing.T, l net.Listener) chan net.Conn {
	got := make(chan net.Conn, 1)
	go func() {
		c, err := l.Accept()
		if err != nil {
			close(got)
			return
		}
		t.Cleanup(func() { c.Close() })
		got <- c
	}()
	return got
}

func TestTheShellStructuresMatchWindows(t *testing.T) {
	var d notifyIconData
	for _, c := range []struct {
		name      string
		got, want uintptr
	}{
		{"NOTIFYICONDATAW", unsafe.Sizeof(d), 976},
		{"hIcon", unsafe.Offsetof(d.Icon), 32},
		{"szTip", unsafe.Offsetof(d.Tip), 40},
		{"szInfo", unsafe.Offsetof(d.Info), 304},
		{"uVersion", unsafe.Offsetof(d.Version), 816},
		{"szInfoTitle", unsafe.Offsetof(d.InfoTitle), 820},
		{"guidItem", unsafe.Offsetof(d.GuidItem), 952},
		{"hBalloonIcon", unsafe.Offsetof(d.BalloonIcon), 968},
		{"WNDCLASSEXW", unsafe.Sizeof(wndClassEx{}), 80},
		{"MSG", unsafe.Sizeof(msg{}), 48},
	} {
		if c.got != c.want {
			t.Errorf("%s: %d, Windows has %d", c.name, c.got, c.want)
		}
	}
}

func TestEveryIconIsMadeAtTheAskedSize(t *testing.T) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	gdi32 := windows.NewLazySystemDLL("gdi32.dll")
	getIconInfo, getObject, deleteObject := user32.NewProc("GetIconInfo"), gdi32.NewProc("GetObjectW"), gdi32.NewProc("DeleteObject")
	for _, name := range []string{"on", "off", "busy", "alert"} {
		for _, size := range []int{16, 20, 24, 32, 40, 48, 64, 80, 256} {
			h, err := iconFile(name, size)
			if err != nil {
				t.Fatalf("%s at %d: %v", name, size, err)
			}
			var info struct {
				Icon         int32
				X, Y         uint32
				Mask, Colors windows.Handle
			}
			getIconInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&info)))
			var bm struct {
				Type, Width, Height, Stride int32
				Planes, Bits                uint16
				Bits0                       uintptr
			}
			getObject.Call(uintptr(info.Colors), unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm)))
			deleteObject.Call(uintptr(info.Mask))
			deleteObject.Call(uintptr(info.Colors))
			release(h)
			if int(bm.Width) != size || int(bm.Height) != size || bm.Bits != 32 {
				t.Errorf("%s at %d: %dx%d, %d bits", name, size, bm.Width, bm.Height, bm.Bits)
			}
		}
	}
	data, _ := art.ReadFile("icons/on.ico")
	for _, broken := range [][]byte{nil, data[:5], data[:20], append(append([]byte{}, data[:6]...), make([]byte, 16)...)} {
		if h, err := makeIcon(broken, 16); err == nil || h != 0 {
			t.Errorf("a broken icon of %d bytes was taken", len(broken))
		}
	}
}

func asPerson(t *testing.T, do func()) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var own windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY, &own); err != nil {
		t.Fatal(err)
	}
	defer own.Close()
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		t.Fatal(err)
	}
	drop := windows.SIDAndAttributes{Sid: admins}
	var restricted windows.Token
	if r, _, err := windows.NewLazySystemDLL("advapi32.dll").NewProc("CreateRestrictedToken").Call(uintptr(own), 0, 1, uintptr(unsafe.Pointer(&drop)), 0, 0, 0, 0, uintptr(unsafe.Pointer(&restricted))); r == 0 {
		t.Fatal(err)
	}
	defer restricted.Close()
	var person windows.Token
	if err := windows.DuplicateTokenEx(restricted, windows.TOKEN_IMPERSONATE|windows.TOKEN_QUERY, nil, windows.SecurityImpersonation, windows.TokenImpersonation, &person); err != nil {
		t.Fatal(err)
	}
	defer person.Close()
	if err := windows.SetThreadToken(nil, person); err != nil {
		t.Fatal(err)
	}
	defer windows.RevertToSelf()
	do()
}

func TestTheTrayAsksOnlyForWhatAPersonGets(t *testing.T) {
	m := regexp.MustCompile(`\(A;;0x([0-9a-f]+);;;IU\)`).FindStringSubmatch(ipc.PipeAccess)
	if m == nil {
		t.Fatalf("the service gives people no rights it names: %s", ipc.PipeAccess)
	}
	given, _ := strconv.ParseUint(m[1], 16, 32)
	if pipeAccess&^given != 0 {
		t.Fatalf("the tray asks for %#x, a person at the computer gets %#x", pipeAccess, given)
	}
	path, l := listen(t, fmt.Sprintf("O:BAD:P(A;;GA;;;BA)(A;;%#x;;;WD)", given))
	got := accept(t, l)
	name, _ := windows.UTF16PtrFromString(path)
	var f *pipe
	var full, err error
	asPerson(t, func() {
		var h windows.Handle
		if h, full = windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0); full == nil {
			windows.CloseHandle(h)
		}
		f, err = dial(context.Background(), path)
	})
	if !errors.Is(full, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("a person was not held to the rights of a person (%v): the test proves nothing", full)
	}
	if err != nil {
		t.Fatalf("the tray cannot open a pipe a person at the computer may open: %v", err)
	}
	defer f.Close()
	c := <-got
	c.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := f.Write([]byte("ping\n")); err != nil {
		t.Fatalf("the tray cannot write: %v", err)
	}
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil || line != "ping\n" {
		t.Fatalf("the service read %q, %v", line, err)
	}
	c.Write([]byte("pong\n"))
	back := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(f).ReadString('\n')
		back <- line
	}()
	select {
	case line := <-back:
		if line != "pong\n" {
			t.Fatalf("the tray read %q", line)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the tray did not read the answer")
	}
}

func TestAClosedPipeStopsAWaitingReadAndRefusesMore(t *testing.T) {
	path, l := listen(t, "O:BAD:P(A;;GA;;;WD)")
	got := accept(t, l)
	f, err := dial(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	c := <-got
	read := make(chan error, 1)
	go func() {
		_, err := f.Read(make([]byte, 64))
		read <- err
	}()
	time.Sleep(100 * time.Millisecond)
	at := time.Now()
	f.Close()
	select {
	case err := <-read:
		if !errors.Is(err, net.ErrClosed) {
			t.Errorf("the waiting read ended with %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a close does not stop a waiting read")
	}
	t.Logf("the read let go in %v", time.Since(at))
	if _, err := f.Write([]byte("x\n")); !errors.Is(err, net.ErrClosed) {
		t.Errorf("a write after close: %v", err)
	}
	f.Close()
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Read(make([]byte, 8)); err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
		t.Errorf("the service still sees the tray: %v", err)
	}
	g := accept(t, l)
	f, err = dial(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	c = <-g
	c.Close()
	if _, err := f.Read(make([]byte, 8)); err != io.EOF {
		t.Errorf("the service went away, the read says %v", err)
	}
	f.Close()
}

func TestTheTrayRefusesAPipeOfSomeoneElse(t *testing.T) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if user.User.Sid.IsWellKnown(windows.WinLocalSystemSid) {
		t.Skip("running as the system")
	}
	path, l := listen(t, "O:"+user.User.Sid.String()+"D:P(A;;GA;;;WD)")
	accept(t, l)
	if f, err := dial(context.Background(), path); !errors.Is(err, errStranger) {
		if f != nil {
			f.Close()
		}
		t.Fatalf("a pipe of a person was taken for the service: %v", err)
	}
}

type fake struct {
	t *testing.T
	c net.Conn
	r *bufio.Reader
}

func (f *fake) read() map[string]any {
	f.t.Helper()
	f.c.SetReadDeadline(time.Now().Add(10 * time.Second))
	line, err := f.r.ReadString('\n')
	if err != nil {
		f.t.Fatalf("the tray sent nothing: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		f.t.Fatalf("the tray sent %q", line)
	}
	return m
}

func (f *fake) send(lines ...string) {
	for _, line := range lines {
		f.c.Write([]byte(line + "\n"))
	}
}

const snapshot = `{"re":%v,"ok":true,"snapshot":{"version":"0.1.0","protocol":1,"connection":{"state":"connected","choice":"auto","server":"nl","delayMs":40,"code":"","error":""},"subscription":{"title":"t","upload":0,"download":0,"total":0,"expire":0,"supportUrl":"","webPageUrl":"","announce":"","fetchedAt":0,"updateEvery":3600,"stop":"","notes":[]},"servers":[{"id":"nl","name":"n","label":"(NL)","country":"NL","protocol":"vless","tier":0,"delayMs":40,"measuredAt":0}],"settings":{"language":"ru"}}}`

func TestTheTrayFollowsTheServiceAndLetsGo(t *testing.T) {
	path, l := listen(t, "O:BAD:P(A;;GA;;;WD)")
	woke := make(chan struct{}, 64)
	s := &shell{wake: func() {
		select {
		case woke <- struct{}{}:
		default:
		}
	}}
	view := func(want func(tray.View) bool) tray.View {
		t.Helper()
		limit := time.After(10 * time.Second)
		for {
			s.mu.Lock()
			v := s.model.View()
			s.mu.Unlock()
			if want(v) {
				return v
			}
			select {
			case <-woke:
			case <-limit:
				t.Fatalf("the tray stays at %+v", v)
			}
		}
	}
	if s.ask("connect", tray.Connect) != 0 {
		t.Fatal("a command was queued without the service")
	}
	got := accept(t, l)
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan struct{})
	go func() {
		defer close(served)
		s.serve(ctx, path)
	}()
	defer func() {
		cancel()
		<-served
	}()
	c := &fake{t: t, c: <-got}
	c.r = bufio.NewReader(c.c)
	hello := c.read()
	if hello["op"] != "hello" || len(hello) != 2 {
		t.Fatalf("the first request is %v", hello)
	}
	c.send(`{"ev":"connection","connection":{"state":"idle","choice":"auto","server":"","delayMs":0,"code":"","error":""}}`, fmt.Sprintf(snapshot, hello["id"]))
	if v := view(func(v tray.View) bool { return v.Icon == tray.On }); v.Status != "VPN работает · (NL)" {
		t.Fatalf("status %q", v.Status)
	}
	id := s.ask("disconnect", tray.Disconnect)
	asked := c.read()
	if id == 0 || asked["op"] != "disconnect" || asked["id"] != float64(id) {
		t.Fatalf("asked %v as %d", asked, id)
	}
	c.send(`{"ev":"connection","connection":{"state":"idle","choice":"auto","server":"","delayMs":0,"code":"","error":""}}`, fmt.Sprintf(`{"re":%d,"ok":true}`, id))
	view(func(v tray.View) bool { return v.Icon == tray.Off && v.Toggle.Action == tray.Connect })
	limit := time.After(10 * time.Second)
	for {
		s.mu.Lock()
		answered := slices.Contains(s.answered, id)
		s.mu.Unlock()
		if answered {
			break
		}
		select {
		case <-woke:
		case <-limit:
			t.Fatal("the answer to the tray's command was not passed on")
		}
	}

	again := accept(t, l)
	c.c.Close()
	view(func(v tray.View) bool { return v.Status == "Служба Spofy не отвечает" && v.Toggle.Grey })
	if s.ask("connect", tray.Connect) != 0 {
		t.Fatal("a command was queued for a service that is gone")
	}
	var second net.Conn
	select {
	case second = <-again:
	case <-time.After(5 * time.Second):
		t.Fatal("the tray did not come back to the service")
	}
	c = &fake{t: t, c: second, r: bufio.NewReader(second)}
	hello = c.read()
	c.send(fmt.Sprintf(snapshot, hello["id"]))
	view(func(v tray.View) bool { return v.Icon == tray.On })
	c.send("this is not a message")
	view(func(v tray.View) bool { return !strings.HasPrefix(v.Status, "VPN") })

	third := accept(t, l)
	var c3 net.Conn
	select {
	case c3 = <-third:
	case <-time.After(5 * time.Second):
		t.Fatal("after a broken line the tray did not open a fresh channel")
	}
	at := time.Now()
	cancel()
	select {
	case <-served:
	case <-time.After(3 * time.Second):
		t.Fatal("the tray does not let the channel go when it quits")
	}
	c3.SetReadDeadline(time.Now().Add(5 * time.Second))
	rest := make([]byte, 4096)
	for {
		if _, err := c3.Read(rest); err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				t.Fatal("the channel stayed open after the tray quit")
			}
			break
		}
	}
	t.Logf("let go in %v", time.Since(at))
}
