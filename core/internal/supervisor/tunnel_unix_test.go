//go:build unix

package supervisor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/engine"
	"github.com/SpofyJet/spofy-app/core/internal/host"
)

func tunnelEngine() {
	in := host.Stdin()
	say := func(r engine.Reply) {
		b, _ := json.Marshal(r)
		os.Stdout.Write(append(b, '\n'))
	}
	say(engine.Reply{Ev: "ready"})
	sc := bufio.NewScanner(in)
	for sc.Scan() {
		var req engine.Request
		json.Unmarshal(sc.Bytes(), &req)
		if req.Op != "start" {
			say(engine.Reply{Re: req.ID, OK: true})
			continue
		}
		box, _ := in.(*host.Inbox)
		var tun *os.File
		if req.Tun && box != nil {
			tun = box.TakeFile("tun")
		}
		switch {
		case tun == nil:
			say(engine.Reply{Re: req.ID, Code: engine.CodeStart, Error: "no tunnel"})
		case bytes.Contains(req.Config, []byte("refuse")):
			say(engine.Reply{Re: req.ID, Code: engine.CodeConfig, Error: "refused"})
		default:
			fmt.Fprintf(tun, "%d %s\n", os.Getpid(), req.Config)
			tun.Close()
			say(engine.Reply{Re: req.ID, OK: true})
		}
	}
}

func TestTunnelDescriptorGoesWithEveryStart(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	lines := bufio.NewReader(r)
	got := func() (int, string) {
		t.Helper()
		r.SetReadDeadline(time.Now().Add(5 * time.Second))
		line, err := lines.ReadString('\n')
		if err != nil {
			t.Fatalf("the engine wrote nothing to the tunnel: %v", err)
		}
		pid, cfg, _ := strings.Cut(strings.TrimSpace(line), " ")
		n, _ := strconv.Atoi(pid)
		return n, cfg
	}
	tun := w
	ev := newEvents()
	s := New(Options{Command: child("tunnel"), Tunnel: func() *os.File { return tun }, OnEvent: ev.on})
	defer s.Close()
	if err := s.Warm(); err != nil {
		t.Fatal(err)
	}
	warm := s.Pid()
	if _, err := s.Start([]byte(`"one"`)); err != nil {
		t.Fatal(err)
	}
	if pid, cfg := got(); pid != warm || cfg != `"one"` {
		t.Fatalf("first start: pid %d (warm %d), config %s", pid, warm, cfg)
	}
	if _, err := s.Start([]byte(`"two"`)); err != nil {
		t.Fatal(err)
	}
	if pid, cfg := got(); pid != warm || cfg != `"two"` {
		t.Fatalf("switch: pid %d (warm %d), config %s", pid, warm, cfg)
	}
	kill(warm)
	ev.wait(t, KindRestored, 10*time.Second)
	if pid, cfg := got(); pid == warm || pid != s.Pid() || cfg != `"two"` {
		t.Fatalf("after a crash: pid %d (was %d, now %d), config %s", pid, warm, s.Pid(), cfg)
	}
	second := s.Pid()
	var e *engine.Error
	if _, err := s.Start([]byte(`"refuse"`)); !errors.As(err, &e) || e.Code != engine.CodeConfig {
		t.Fatalf("refused start: %v", err)
	}
	if !gone(second) {
		t.Fatal("the engine that refused a start with a descriptor is still running")
	}
	if _, err := s.Start([]byte(`"three"`)); err != nil {
		t.Fatal(err)
	}
	if pid, cfg := got(); pid == second || cfg != `"three"` {
		t.Fatalf("after a refusal: pid %d (was %d), config %s", pid, second, cfg)
	}
	tun = nil
	if _, err := s.Start([]byte(`"four"`)); !errors.As(err, &e) || e.Code != CodeGuard {
		t.Fatalf("start without a tunnel: %v", err)
	}
}
