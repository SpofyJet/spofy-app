package sysproxy

import (
	"context"
	"os/exec"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type ieConfig struct {
	AutoDetect int32
	Script     *uint16
	Proxy      *uint16
	Bypass     *uint16
}

func winhttp(t *testing.T) (proxy, bypass string) {
	t.Helper()
	var c ieConfig
	r, _, err := windows.NewLazySystemDLL("winhttp.dll").NewProc("WinHttpGetIEProxyConfigForCurrentUser").Call(uintptr(unsafe.Pointer(&c)))
	if r == 0 {
		t.Fatalf("WinHTTP: %v", err)
	}
	take(c.Script)
	return take(c.Proxy), take(c.Bypass)
}

func legacy(t *testing.T) (uint64, string) {
	t.Helper()
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	on, _, _ := k.GetIntegerValue("ProxyEnable")
	server, _, _ := k.GetStringValue("ProxyServer")
	return on, server
}

func powershell(command string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command).Run()
}

func TestTheRealSettingsOfThePersonComeBackExactly(t *testing.T) {
	sys := WinINet{}
	if rec, err := sys.Load(); err != nil || rec != nil {
		t.Skipf("this account already has a Spofy record: %v", err)
	}
	before, err := sys.Get("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sys.Put("", before)
		sys.Forget()
		sys.Notify()
	})
	theirs := Settings{Flags: Direct | Detect, Server: "10.1.2.3:3128", Bypass: "corp.example;<local>", Script: "http://pac.example.test/proxy.pac"}
	if err := sys.Put("", theirs); err != nil {
		t.Fatal(err)
	}
	if got, err := sys.Get(""); err != nil || got != theirs {
		t.Fatalf("the system does not keep what was put: %+v %v", got, err)
	}
	vpn := Settings{Flags: Direct | Manual, Server: "corp.example:8080", Bypass: "<local>"}
	ras := ""
	if powershell("Add-VpnConnection -Name 'Spofy Stand VPN' -ServerAddress 192.0.2.1 -Force") == nil {
		t.Cleanup(func() { powershell("Remove-VpnConnection -Name 'Spofy Stand VPN' -Force") })
		ras = "Spofy Stand VPN"
		if err := sys.Put(ras, vpn); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Log("no dial-up and VPN entries on this machine: only the network settings are checked")
	}
	conns, _ := sys.Connections()
	t.Logf("connections: %q", conns)
	if ras != "" && len(conns) < 2 {
		t.Fatalf("the VPN entry is not listed: %q", conns)
	}
	if err := Apply("127.0.0.1:41234"); err != nil {
		t.Fatal(err)
	}
	for _, c := range conns {
		if got, err := sys.Get(c); err != nil || got != Ours("127.0.0.1:41234") {
			t.Fatalf("%q after apply: %+v %v", c, got, err)
		}
	}
	if on, server := legacy(t); on != 1 || server != "127.0.0.1:41234" {
		t.Fatalf("programs that read the registry see %d %q", on, server)
	}
	if proxy, bypass := winhttp(t); proxy != "127.0.0.1:41234" || bypass != Bypass {
		t.Fatalf("WinHTTP sees %q %q", proxy, bypass)
	}
	if err := Apply("127.0.0.1:41235"); err != nil {
		t.Fatal(err)
	}
	rec, err := sys.Load()
	if err != nil || rec == nil || rec.Previous[""] != theirs || (ras != "" && rec.Previous[ras] != vpn) {
		t.Fatalf("a second apply after a crash forgot the first settings: %+v %v", rec, err)
	}
	if err := Restore(); err != nil {
		t.Fatal(err)
	}
	if got, err := sys.Get(""); err != nil || got != theirs {
		t.Fatalf("after restore: %+v %v", got, err)
	}
	if ras != "" {
		if got, err := sys.Get(ras); err != nil || got != vpn {
			t.Fatalf("%q after restore: %+v %v", ras, got, err)
		}
	}
	if on, server := legacy(t); on != 0 || server == "127.0.0.1:41235" {
		t.Fatalf("the registry after restore: %d %q", on, server)
	}
	if proxy, _ := winhttp(t); proxy == "127.0.0.1:41235" {
		t.Fatalf("WinHTTP after restore sees %q", proxy)
	}
	if rec, err := sys.Load(); err != nil || rec != nil {
		t.Fatalf("the record stays: %+v %v", rec, err)
	}
	if err := Restore(); err != nil {
		t.Fatal(err)
	}
	if got, _ := sys.Get(""); got != theirs {
		t.Fatalf("a second restore changed something: %+v", got)
	}
}
