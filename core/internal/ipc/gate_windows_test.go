package ipc

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/ipc/namedpipe"
)

func TestDialForTheGate(t *testing.T) {
	path := os.Getenv("SPOFY_GATE_PIPE")
	if path == "" {
		t.Skip("a helper of TestTheServiceHearsOnlyItsProgramsTheSystemAndAdministrators")
	}
	c, err := namedpipe.DialTimeout(path, 5*time.Second)
	if err != nil {
		fmt.Println("NO PIPE", err)
		return
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	c.Write([]byte("ping\n"))
	if line, _ := bufio.NewReader(c).ReadString('\n'); line == "pong\n" {
		fmt.Println("HEARD")
		return
	}
	fmt.Println("REFUSED")
}

func person(t *testing.T) windows.Token {
	t.Helper()
	var own windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY|windows.TOKEN_ASSIGN_PRIMARY, &own); err != nil {
		t.Fatal(err)
	}
	defer own.Close()
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		t.Fatal(err)
	}
	drop := windows.SIDAndAttributes{Sid: admins}
	var restricted windows.Token
	if r, _, err := windows.NewLazySystemDLL("advapi32.dll").NewProc("CreateRestrictedToken").Call(uintptr(own), 1, 1, uintptr(unsafe.Pointer(&drop)), 0, 0, 0, 0, uintptr(unsafe.Pointer(&restricted))); r == 0 {
		t.Fatal(err)
	}
	t.Cleanup(func() { restricted.Close() })
	return restricted
}

func TestTheServiceHearsOnlyItsProgramsTheSystemAndAdministrators(t *testing.T) {
	self, err := image(windows.CurrentProcess())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		programs []string
		as       windows.Token
		heard    bool
	}{
		{"another program of a person", []string{filepath.Join(filepath.Dir(self), "spofy.exe")}, person(t), false},
		{"our program run by a person", []string{strings.ToUpper(self)}, person(t), true},
	}
	if empowered(windows.CurrentProcess()) {
		cases = append(cases, struct {
			name     string
			programs []string
			as       windows.Token
			heard    bool
		}{"any program of an administrator", nil, 0, true})
	} else {
		t.Log("the test does not run as an administrator: that case is not checked")
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		path := fmt.Sprintf(`\\.\pipe\spofy-gate-%d-%d`, os.Getpid(), i)
		l, err := (&namedpipe.ListenConfig{SecurityDescriptor: sd}).Listen(path)
		if err != nil {
			t.Fatal(err)
		}
		g := gate{Listener: l, heard: heard(c.programs)}
		served := make(chan struct{})
		go func() {
			defer close(served)
			for {
				conn, err := g.Accept()
				if err != nil {
					return
				}
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				if line, _ := bufio.NewReader(conn).ReadString('\n'); line == "ping\n" {
					conn.Write([]byte("pong\n"))
				}
				conn.Close()
			}
		}()
		cmd := exec.Command(os.Args[0], "-test.run=^TestDialForTheGate$", "-test.count=1")
		cmd.Env = append(os.Environ(), "SPOFY_GATE_PIPE="+path)
		if c.as != 0 {
			cmd.SysProcAttr = &syscall.SysProcAttr{Token: syscall.Token(c.as)}
		}
		out, err := cmd.Output()
		l.Close()
		<-served
		said := string(out)
		if err != nil || strings.Contains(said, "NO PIPE") {
			t.Fatalf("%s: the helper did not reach the pipe: %v %s", c.name, err, said)
		}
		if strings.Contains(said, "HEARD") != c.heard || !strings.Contains(said, "HEARD") && !strings.Contains(said, "REFUSED") {
			t.Fatalf("%s: heard %v, want %v: %s", c.name, strings.Contains(said, "HEARD"), c.heard, said)
		}
	}
}
