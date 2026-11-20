package ipc

import (
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/ipc/namedpipe"
)

var pipes atomic.Int64

func listenChannel(t *testing.T) (net.Listener, string) {
	t.Helper()
	path := fmt.Sprintf(`\\.\pipe\spofy-test-%d-%d`, os.Getpid(), pipes.Add(1))
	l, err := ListenPipe(path)
	if err != nil {
		t.Fatal(err)
	}
	return l, path
}

func dialChannel(path string) (net.Conn, error) {
	return namedpipe.DialTimeout(path, 5*time.Second)
}

func TestPipeCannotBeTakenBySecondServer(t *testing.T) {
	l, path := listenChannel(t)
	defer l.Close()
	if second, err := ListenPipe(path); err == nil {
		second.Close()
		t.Fatal("a second server took the name of a pipe that is already served")
	}
}

func TestPipeLetsInOnlyTheSystemAdministratorsAndPeopleAtTheComputer(t *testing.T) {
	l, path := listenChannel(t)
	defer l.Close()
	go func() {
		if c, err := l.Accept(); err == nil {
			c.Close()
		}
	}()
	c, err := dialChannel(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	h := windows.Handle(c.(interface{ Fd() uintptr }).Fd())
	sd, err := windows.GetSecurityInfo(h, windows.SE_KERNEL_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner.IsWellKnown(windows.WinLocalSystemSid) != system() {
		t.Fatalf("owner %v, the process runs as the system: %v", owner, system())
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("the pipe takes rights from its parent: %s", sd)
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		t.Fatalf("no access list: %v", err)
	}
	const (
		readData           = 0x1
		writeData          = 0x2
		createPipeInstance = 0x4
		fileAll            = 0x1f01ff
	)
	people, _ := windows.CreateWellKnownSid(windows.WinInteractiveSid)
	system, _ := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	admins, _ := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	seen := map[string]uint32{}
	for i := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			t.Fatal(err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			t.Fatalf("entry %d is not an allowing one: %s", i, sd)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		switch {
		case sid.Equals(system):
			seen["system"] = uint32(ace.Mask)
		case sid.Equals(admins):
			seen["admins"] = uint32(ace.Mask)
		case sid.Equals(people):
			seen["people"] = uint32(ace.Mask)
		default:
			t.Errorf("the pipe lets in %s: %s", sid, sd)
		}
	}
	for _, who := range []string{"system", "admins"} {
		if m := seen[who]; m != fileAll && m != windows.GENERIC_ALL {
			t.Errorf("%s: mask %#x, want all: %s", who, m, sd)
		}
	}
	m := seen["people"]
	if m&(readData|writeData) != readData|writeData {
		t.Errorf("people at the computer cannot talk to the core: %#x", m)
	}
	if m&(createPipeInstance|windows.WRITE_DAC|windows.WRITE_OWNER|windows.DELETE) != 0 {
		t.Errorf("people at the computer may serve the pipe themselves or change its rights: %#x", m)
	}
	ui, err := windows.CreateFile(windows.StringToUTF16Ptr(path), windows.FILE_GENERIC_READ|writeData, 0, nil, windows.OPEN_EXISTING, windows.SECURITY_SQOS_PRESENT|windows.SECURITY_IDENTIFICATION, 0)
	if err != nil && err != windows.ERROR_PIPE_BUSY {
		t.Fatalf("the rights the interface asks for are not enough: %v", err)
	}
	if err == nil {
		windows.CloseHandle(ui)
	}
}
