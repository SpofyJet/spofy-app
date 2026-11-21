package app

import (
	"encoding/json"
	"errors"
	"net"
	"reflect"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/SpofyJet/spofy-app/core/internal/session"
	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
)

type pointer struct {
	mu      sync.Mutex
	calls   []string
	running []bool
	tunnel  func() []byte
}

func (p *pointer) Set(address string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "set "+address)
	p.running = append(p.running, xcfg.Local(p.tunnel()))
	return nil
}

func (p *pointer) Clear() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "clear")
	return nil
}

func (p *pointer) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.calls)
}

func proxied(t *testing.T, tune func(*Config)) (*bench, *pointer) {
	t.Helper()
	p := &pointer{}
	b := newBench(t, func(c *Config) {
		c.System = p
		if tune != nil {
			tune(c)
		}
	})
	p.tunnel = b.tunnel.Config
	b.until("the system proxy left by an earlier run is given back at start", func() bool { return len(p.seen()) == 1 })
	return b.subscribed(), p
}

func listening(t *testing.T, cfg []byte) int {
	t.Helper()
	var c struct {
		Inbounds []struct {
			Protocol string `json:"protocol"`
			Listen   string `json:"listen"`
			Port     int    `json:"port"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(cfg, &c); err != nil || len(c.Inbounds) != 1 || c.Inbounds[0].Protocol != "mixed" || c.Inbounds[0].Listen != "127.0.0.1" {
		t.Fatalf("not the local proxy: %v %s", err, cfg)
	}
	return c.Inbounds[0].Port
}

func TestProxyModeIsOnlyWhereTheSystemCanPointProgramsAtIt(t *testing.T) {
	b := newBench(t, nil)
	if err := b.app.Set(KeyMode, json.RawMessage(`"proxy"`)); code(err) != CodeValue || b.app.Snapshot().Settings[KeyMode] != nil {
		t.Fatalf("without a system proxy: %v %s", err, b.app.Snapshot().Settings[KeyMode])
	}
	b, _ = proxied(t, nil)
	for _, bad := range []string{`"tun"`, `"Proxy"`, `true`, `1`} {
		if err := b.app.Set(KeyMode, json.RawMessage(bad)); code(err) != CodeValue {
			t.Fatalf("%s: %v", bad, err)
		}
	}
	for _, bad := range []string{`80`, `65536`, `"8080"`, `1080.5`} {
		if err := b.app.Set(KeyProxyPort, json.RawMessage(bad)); code(err) != CodeValue {
			t.Fatalf("port %s: %v", bad, err)
		}
	}
	if err := b.app.Set(KeyMode, json.RawMessage(`"proxy"`)); err != nil || string(b.app.Snapshot().Settings[KeyMode]) != `"proxy"` {
		t.Fatalf("%v %s", err, b.app.Snapshot().Settings[KeyMode])
	}
	if err := b.app.Set(KeyMode, nil); err != nil || b.app.Snapshot().Settings[KeyMode] != nil {
		t.Fatal(err)
	}
}

func TestProxyModePointsTheSystemAtAPortThatStaysAndGivesItBack(t *testing.T) {
	b, p := proxied(t, nil)
	if err := b.app.Set(KeyMode, json.RawMessage(`"proxy"`)); err != nil {
		t.Fatal(err)
	}
	if err := b.app.Connect(Auto); err != nil {
		t.Fatal(err)
	}
	port := listening(t, b.tunnel.Config())
	address := "127.0.0.1:" + strconv.Itoa(port)
	if saved := string(b.app.Snapshot().Settings[KeyProxyPort]); saved != strconv.Itoa(port) || port < 1024 {
		t.Fatalf("port %d, saved %s", port, saved)
	}
	b.until("the system is pointed at the port once the server answered", func() bool { return len(p.seen()) == 2 })
	if got := p.seen(); got[1] != "set "+address || !p.running[0] {
		t.Fatalf("calls %v, the local proxy ran when the system was pointed at it: %v", got, p.running)
	}
	if err := b.app.Connect("de"); err != nil || listening(t, b.tunnel.Config()) != port {
		t.Fatalf("another server, another port: %v %s", err, b.tunnel.Config())
	}
	if err := b.app.Disconnect(); err != nil {
		t.Fatal(err)
	}
	b.until("given back after a disconnect", func() bool { return len(p.seen()) == 3 && p.seen()[2] == "clear" })
	if err := b.app.Connect(Auto); err != nil || listening(t, b.tunnel.Config()) != port {
		t.Fatalf("the next connection takes the same port: %v %s", err, b.tunnel.Config())
	}
	b.until("pointed again", func() bool { return len(p.seen()) == 4 })
	if err := b.app.Disconnect(); err != nil {
		t.Fatal(err)
	}
	taken, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	if err := b.app.Connect(Auto); err != nil {
		t.Fatal(err)
	}
	moved := listening(t, b.tunnel.Config())
	if moved == port || string(b.app.Snapshot().Settings[KeyProxyPort]) != strconv.Itoa(moved) {
		t.Fatalf("a port taken by another program is not used: %d", moved)
	}
	b.until("pointed at the new port", func() bool { return slices.Contains(p.seen(), "set 127.0.0.1:"+strconv.Itoa(moved)) })
	b.app.Close()
	if got := p.seen(); got[len(got)-1] != "clear" {
		t.Fatalf("the service stops: the system proxy is given back before it is gone: %v", got)
	}
}

func TestTheBlockDoesNotApplyToTheProxyMode(t *testing.T) {
	var kept []bool
	b, p := proxied(t, func(c *Config) { c.Keep = func(on bool) { kept = append(kept, on) } })
	if err := b.app.Set(KeyKillSwitch, json.RawMessage(`true`)); err != nil {
		t.Fatal(err)
	}
	if err := b.app.Set(KeyMode, json.RawMessage(`"proxy"`)); err != nil {
		t.Fatal(err)
	}
	b.tunnel.Answer(nil)
	if err := b.app.Connect(Auto); code(err) != session.CodeNoServer || b.tunnel.Running() != "" || b.app.Snapshot().Connection.State != session.Failed {
		t.Fatalf("no server answers: err %v, running %q, state %s", err, b.tunnel.Running(), b.app.Snapshot().Connection.State)
	}
	if err := b.app.Set(KeyMode, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kept, []bool{true, false, true}) {
		t.Fatalf("kept %v", kept)
	}
	if err := b.app.Connect(Auto); code(err) != session.CodeNoServer || b.tunnel.Running() == "" || b.app.Snapshot().Connection.State != session.Stalled {
		t.Fatalf("back in the tunnel the block holds: err %v, running %q", err, b.tunnel.Running())
	}
	if slices.ContainsFunc(p.seen(), func(c string) bool { return c != "clear" }) {
		t.Fatalf("the system was pointed at a proxy that never answered: %v", p.seen())
	}
}

func TestSwitchingTheModeWhileConnected(t *testing.T) {
	b, p := proxied(t, nil)
	if err := b.app.Connect(Auto); err != nil || xcfg.Local(b.tunnel.Config()) {
		t.Fatalf("%v %s", err, b.tunnel.Config())
	}
	if err := b.app.Set(KeyMode, json.RawMessage(`"proxy"`)); err != nil {
		t.Fatal(err)
	}
	b.until("reconnected through the local proxy and the system pointed at it", func() bool { return len(p.seen()) == 2 })
	if !xcfg.Local(b.tunnel.Config()) || !p.running[0] || b.app.Snapshot().Connection.State != session.Connected {
		t.Fatalf("config %s, running when pointed %v", b.tunnel.Config(), p.running)
	}
	if err := b.app.Set(KeyMode, nil); err != nil {
		t.Fatal(err)
	}
	b.until("given back as soon as the mode is off", func() bool { return len(p.seen()) == 3 })
	b.until("reconnected without the local proxy", func() bool { return !xcfg.Local(b.tunnel.Config()) })
	if got := p.seen(); got[2] != "clear" {
		t.Fatalf("calls %v", got)
	}
}

type refusing struct{ pointer }

func (r *refusing) Set(string) error { return errors.New("no session") }

func TestASystemThatRefusesIsTriedAgainOnTheNextChange(t *testing.T) {
	r := &refusing{}
	b := newBench(t, func(c *Config) { c.System = r })
	r.tunnel = b.tunnel.Config
	b.subscribed()
	b.app.Set(KeyMode, json.RawMessage(`"proxy"`))
	if err := b.app.Connect(Auto); err != nil {
		t.Fatal(err)
	}
	b.until("the refusal is in the log", func() bool {
		for _, l := range b.app.Logs() {
			if l.Text == "system proxy: no session" {
				return true
			}
		}
		return false
	})
	if err := b.app.Disconnect(); err != nil {
		t.Fatal(err)
	}
	b.until("given back", func() bool { return slices.Contains(r.seen(), "clear") })
}
