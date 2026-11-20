package supervisor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/engine"
	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
)

const modeEnv = "SPOFY_TEST_ENGINE"

type exitAfterStart struct{ w io.Writer }

func (e exitAfterStart) Write(b []byte) (int, error) {
	n, err := e.w.Write(b)
	if bytes.Contains(b, []byte(`"re":`)) && bytes.Contains(b, []byte(`"ok":true`)) {
		time.AfterFunc(30*time.Millisecond, func() { os.Exit(3) })
	}
	return n, err
}

func TestMain(m *testing.M) {
	switch os.Getenv(modeEnv) {
	case "serve":
		engine.Serve(os.Stdin, os.Stdout)
		os.Exit(0)
	case "flaky":
		engine.Serve(os.Stdin, exitAfterStart{os.Stdout})
		os.Exit(0)
	case "crashy":
		pr, pw := io.Pipe()
		go func() {
			sc := bufio.NewScanner(os.Stdin)
			sc.Buffer(make([]byte, 64<<10), 4<<20)
			for sc.Scan() {
				if bytes.Contains(sc.Bytes(), []byte(`"tag":"boom"`)) {
					os.Exit(7)
				}
				pw.Write(append(bytes.Clone(sc.Bytes()), '\n'))
			}
			pw.Close()
		}()
		engine.Serve(pr, os.Stdout)
		os.Exit(0)
	case "tunnel":
		tunnelEngine()
		os.Exit(0)
	case "parent":
		cfg, _ := os.ReadFile(os.Getenv("SPOFY_TEST_CONFIG"))
		s := New(Options{Command: child("serve")})
		if _, err := s.Start(cfg); err != nil {
			fmt.Println("error", err)
			os.Exit(1)
		}
		fmt.Println(s.Pid())
		select {}
	}
	os.Exit(m.Run())
}

func child(mode string) func() *exec.Cmd {
	return func() *exec.Cmd {
		cmd := exec.Command(os.Args[0])
		cmd.Env = append(os.Environ(), modeEnv+"="+mode)
		return cmd
	}
}

func config(t *testing.T) ([]byte, int) {
	t.Helper()
	body, err := os.ReadFile("../../testdata/sub/sub.json")
	if err != nil {
		t.Fatal(err)
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(body, &raws); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	cfg, err := xcfg.Build(raws[0], xcfg.Options{Mode: xcfg.ModeProxy, Proxy: xcfg.Proxy{Port: port, User: "u", Pass: "p"}})
	if err != nil {
		t.Fatal(err)
	}
	return cfg, port
}

func listening(port int) bool {
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 200*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

type events struct {
	mu   sync.Mutex
	list []Event
	ch   chan Event
}

func newEvents() *events { return &events{ch: make(chan Event, 1024)} }

func (e *events) on(ev Event) {
	if ev.Kind == KindLog {
		return
	}
	e.mu.Lock()
	e.list = append(e.list, ev)
	e.mu.Unlock()
	e.ch <- ev
}

func (e *events) wait(t *testing.T, kind string, d time.Duration) Event {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case ev := <-e.ch:
			if ev.Kind == kind {
				return ev
			}
		case <-deadline:
			t.Fatalf("no %q event within %v; got %+v", kind, d, e.kinds())
		}
	}
}

func (e *events) kinds() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for _, ev := range e.list {
		out = append(out, ev.Kind)
	}
	return out
}

func TestStartTrafficStop(t *testing.T) {
	cfg, port := config(t)
	s := New(Options{Command: child("serve")})
	defer s.Close()
	took, err := s.Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cold start (spawn + core start): %v", took)
	if !listening(port) {
		t.Fatal("inbound is not listening after start")
	}
	if _, err := s.Traffic(xcfg.TagProxy); err != nil {
		t.Fatal(err)
	}
	warm, err := s.Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("warm restart (same process): %v", warm)
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	if listening(port) {
		t.Fatal("inbound still listening after stop")
	}
	if s.Pid() == 0 {
		t.Fatal("engine process should stay warm after stop")
	}
}

func TestRecoversFromCrash(t *testing.T) {
	cfg, port := config(t)
	ev := newEvents()
	s := New(Options{Command: child("serve"), OnEvent: ev.on})
	defer s.Close()
	if _, err := s.Start(cfg); err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 3; round++ {
		pid := s.Pid()
		kill(pid)
		ev.wait(t, KindCrashed, 3*time.Second)
		restored := ev.wait(t, KindRestored, 5*time.Second)
		t.Logf("round %d: downtime %v", round, restored.Downtime)
		if restored.Downtime > 2*time.Second {
			t.Fatalf("round %d: downtime %v is too long", round, restored.Downtime)
		}
		if next := s.Pid(); next == 0 || next == pid {
			t.Fatalf("round %d: pid %d -> %d", round, pid, next)
		}
		if !listening(port) {
			t.Fatalf("round %d: inbound is not back", round)
		}
	}
}

func TestConfigErrorIsNotRetried(t *testing.T) {
	ev := newEvents()
	s := New(Options{Command: child("serve"), OnEvent: ev.on})
	defer s.Close()
	_, err := s.Start([]byte(`{"outbounds":[{"protocol":"nope"}]}`))
	var e *engine.Error
	if !errors.As(err, &e) || e.Code != engine.CodeConfig {
		t.Fatalf("err = %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if got := ev.kinds(); len(got) != 0 {
		t.Fatalf("unexpected events: %v", got)
	}
}

func TestCrashLoopGivesUp(t *testing.T) {
	cfg, port := config(t)
	ev := newEvents()
	s := New(Options{Command: child("flaky"), OnEvent: ev.on, Backoff: []time.Duration{0}, CrashLimit: 4})
	defer s.Close()
	if _, err := s.Start(cfg); err != nil {
		t.Fatal(err)
	}
	failed := ev.wait(t, KindFailed, 15*time.Second)
	if failed.Code != CodeCrashLoop {
		t.Fatalf("failed = %+v", failed)
	}
	time.Sleep(300 * time.Millisecond)
	if listening(port) {
		t.Fatal("engine was restarted after giving up")
	}
	crashed := 0
	for _, k := range ev.kinds() {
		if k == KindCrashed {
			crashed++
		}
	}
	if crashed != 4 {
		t.Fatalf("crashed events = %d, want 4 (%v)", crashed, ev.kinds())
	}
}

func TestStopDuringRecoveryStaysStopped(t *testing.T) {
	cfg, port := config(t)
	ev := newEvents()
	s := New(Options{Command: child("flaky"), OnEvent: ev.on, Backoff: []time.Duration{400 * time.Millisecond}, CrashLimit: 50})
	defer s.Close()
	if _, err := s.Start(cfg); err != nil {
		t.Fatal(err)
	}
	ev.wait(t, KindCrashed, 3*time.Second)
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)
	if listening(port) {
		t.Fatal("engine came back after stop")
	}
	for _, k := range ev.kinds() {
		if k == KindRestored {
			t.Fatalf("restored after stop: %v", ev.kinds())
		}
	}
}

func TestCloseLeavesNoProcess(t *testing.T) {
	cfg, port := config(t)
	s := New(Options{Command: child("serve")})
	if _, err := s.Start(cfg); err != nil {
		t.Fatal(err)
	}
	pid := s.Pid()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if !gone(pid) {
		t.Fatalf("engine %d is still alive after close", pid)
	}
	if listening(port) {
		t.Fatal("inbound still listening after close")
	}
}

func TestEngineDiesWithParent(t *testing.T) {
	cfg, port := config(t)
	path := t.TempDir() + "/cfg.json"
	if err := os.WriteFile(path, cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	parent := exec.Command(os.Args[0])
	parent.Env = append(os.Environ(), modeEnv+"=parent", "SPOFY_TEST_CONFIG="+path)
	out, err := parent.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	defer parent.Process.Kill()
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("parent said %q", line)
	}
	if !listening(port) {
		t.Fatal("engine under helper parent is not listening")
	}
	parent.Process.Kill()
	parent.Wait()
	deadline := time.Now().Add(3 * time.Second)
	for !gone(pid) {
		if time.Now().After(deadline) {
			kill(pid)
			t.Fatalf("engine %d outlived its parent", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if listening(port) {
		t.Fatal("inbound still listening after parent death")
	}
}

type fakeGuard struct {
	mu     sync.Mutex
	calls  []string
	up     bool
	port   int
	early  bool
	failUp bool
}

func (g *fakeGuard) Raise() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls = append(g.calls, "raise")
	if g.failUp {
		return errors.New("no permission")
	}
	if !g.up && listening(g.port) {
		g.early = true
	}
	g.up = true
	return nil
}

func (g *fakeGuard) Lower() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls = append(g.calls, "lower")
	g.up = false
	return nil
}

func (g *fakeGuard) state() (bool, int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	lowered := 0
	for _, c := range g.calls {
		if c == "lower" {
			lowered++
		}
	}
	return g.up, lowered
}

func TestGuardWrapsTheSession(t *testing.T) {
	cfg, port := config(t)
	g := &fakeGuard{port: port}
	s := New(Options{Command: child("serve"), Guard: g})
	defer s.Close()
	if _, err := s.Start(cfg); err != nil {
		t.Fatal(err)
	}
	if up, lowered := g.state(); !up || lowered != 0 || g.early {
		t.Fatalf("after start: up=%v lowered=%d engineRanBeforeGuard=%v", up, lowered, g.early)
	}
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	if up, _ := g.state(); up {
		t.Fatal("guard is still up after stop")
	}
}

func TestGuardStaysUpWhileEngineRestarts(t *testing.T) {
	cfg, port := config(t)
	g := &fakeGuard{port: port}
	ev := newEvents()
	s := New(Options{Command: child("serve"), Guard: g, OnEvent: ev.on})
	defer s.Close()
	if _, err := s.Start(cfg); err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 3; round++ {
		kill(s.Pid())
		ev.wait(t, KindRestored, 5*time.Second)
		if up, lowered := g.state(); !up || lowered != 0 {
			t.Fatalf("round %d: up=%v lowered=%d", round, up, lowered)
		}
	}
}

func TestGuardLoweredWhenStartFails(t *testing.T) {
	g := &fakeGuard{}
	s := New(Options{Command: child("serve"), Guard: g})
	defer s.Close()
	if _, err := s.Start([]byte(`{"outbounds":[{"protocol":"nope"}]}`)); err == nil {
		t.Fatal("start succeeded")
	}
	if up, lowered := g.state(); up || lowered != 1 {
		t.Fatalf("up=%v lowered=%d", up, lowered)
	}
}

func TestGuardHeldWhenSwitchFails(t *testing.T) {
	cfg, _ := config(t)
	g := &fakeGuard{}
	s := New(Options{Command: child("serve"), Guard: g})
	defer s.Close()
	if _, err := s.Start(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start([]byte(`{"outbounds":[{"protocol":"nope"}]}`)); err == nil {
		t.Fatal("start succeeded")
	}
	if up, lowered := g.state(); !up || lowered != 0 {
		t.Fatalf("after a failed switch: up=%v lowered=%d", up, lowered)
	}
	if _, err := s.Start(cfg); err != nil {
		t.Fatal(err)
	}
	if up, lowered := g.state(); !up || lowered != 0 {
		t.Fatalf("after the next server: up=%v lowered=%d", up, lowered)
	}
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	if up, lowered := g.state(); up || lowered != 1 {
		t.Fatalf("after stop: up=%v lowered=%d", up, lowered)
	}
}

func TestGuardFailureStopsStart(t *testing.T) {
	cfg, port := config(t)
	g := &fakeGuard{failUp: true}
	s := New(Options{Command: child("serve"), Guard: g})
	defer s.Close()
	_, err := s.Start(cfg)
	var e *engine.Error
	if !errors.As(err, &e) || e.Code != CodeGuard {
		t.Fatalf("err = %v", err)
	}
	if listening(port) {
		t.Fatal("engine started without the guard")
	}
}

func TestGuardLoweredAfterCrashLoop(t *testing.T) {
	cfg, _ := config(t)
	g := &fakeGuard{}
	ev := newEvents()
	s := New(Options{Command: child("flaky"), Guard: g, OnEvent: ev.on, Backoff: []time.Duration{0}, CrashLimit: 3})
	defer s.Close()
	if _, err := s.Start(cfg); err != nil {
		t.Fatal(err)
	}
	ev.wait(t, KindFailed, 15*time.Second)
	if up, _ := g.state(); up {
		t.Fatal("guard is still up after giving up")
	}
}

func TestGuardHeldAfterCrashLoopInStrictMode(t *testing.T) {
	cfg, _ := config(t)
	g := &fakeGuard{}
	ev := newEvents()
	s := New(Options{Command: child("flaky"), Guard: g, HoldGuard: true, OnEvent: ev.on, Backoff: []time.Duration{0}, CrashLimit: 3})
	defer s.Close()
	if _, err := s.Start(cfg); err != nil {
		t.Fatal(err)
	}
	ev.wait(t, KindFailed, 15*time.Second)
	if up, lowered := g.state(); !up || lowered != 0 {
		t.Fatalf("up=%v lowered=%d", up, lowered)
	}
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	if up, _ := g.state(); up {
		t.Fatal("guard is still up after the user stopped")
	}
}
