package supervisor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/engine"
)

const direct = `{"log":{"loglevel":"none"},"outbounds":[{"tag":"proxy","protocol":"freedom"},{"tag":"dead","protocol":"blackhole"}]}`

func target(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/delay":
			time.Sleep(500 * time.Millisecond)
		case "/slow":
			select {
			case <-r.Context().Done():
			case <-time.After(20 * time.Second):
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

type spawned struct {
	mu   sync.Mutex
	cmds []*exec.Cmd
}

func (s *spawned) command(mode string) func() *exec.Cmd {
	return func() *exec.Cmd {
		cmd := child(mode)()
		s.mu.Lock()
		s.cmds = append(s.cmds, cmd)
		s.mu.Unlock()
		return cmd
	}
}

func (s *spawned) allGone(t *testing.T) int {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	deadline := time.Now().Add(3 * time.Second)
	for _, cmd := range s.cmds {
		for cmd.Process != nil && !gone(cmd.Process.Pid) {
			if time.Now().After(deadline) {
				t.Fatalf("prober %d is still alive", cmd.Process.Pid)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	return len(s.cmds)
}

func collect(n int) ([]Measurement, func(int, Measurement)) {
	out := make([]Measurement, n)
	for i := range out {
		out[i].Code = "untouched"
	}
	var mu sync.Mutex
	return out, func(i int, m Measurement) {
		mu.Lock()
		out[i] = m
		mu.Unlock()
	}
}

func code(err error) engine.Code {
	var e *engine.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestProbeLiveEngine(t *testing.T) {
	url := target(t) + "/generate_204"
	s := New(Options{Command: child("serve")})
	defer s.Close()
	if _, err := s.Probe(context.Background(), "proxy", url, time.Second); code(err) != CodeUnavailable {
		t.Fatalf("probe without engine: %v", err)
	}
	if _, err := s.Start([]byte(direct)); err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"", "proxy"} {
		if _, err := s.Probe(context.Background(), tag, url, 5*time.Second); err != nil {
			t.Fatalf("tag %q: %v", tag, err)
		}
	}
	if _, err := s.Probe(context.Background(), "dead", url, 5*time.Second); code(err) != engine.CodeProbe {
		t.Fatalf("dead outbound: %v", err)
	}
	began := time.Now()
	if _, err := s.Probe(context.Background(), "proxy", target(t)+"/slow", 300*time.Millisecond); code(err) != engine.CodeProbe {
		t.Fatalf("slow target: %v", err)
	}
	if took := time.Since(began); took > 2*time.Second {
		t.Fatalf("probe ignored its timeout: %v", took)
	}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	began = time.Now()
	if _, err := s.Probe(ctx, "proxy", target(t)+"/slow", 10*time.Second); code(err) != CodeCanceled {
		t.Fatalf("canceled probe: %v", err)
	}
	if took := time.Since(began); took > 2*time.Second {
		t.Fatalf("probe ignored cancel: %v", took)
	}
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Probe(context.Background(), "proxy", url, time.Second); code(err) != engine.CodeIdle {
		t.Fatalf("probe after stop: %v", err)
	}
}

func TestMeasureInParallel(t *testing.T) {
	base := target(t)
	var procs spawned
	s := New(Options{Command: procs.command("serve")})
	defer s.Close()
	targets := make([]Target, 8)
	for i := range targets {
		targets[i] = Target{Config: []byte(direct), Tag: "proxy"}
	}
	got, each := collect(len(targets))
	began := time.Now()
	s.Measure(context.Background(), targets, MeasureOptions{URL: base + "/delay", Timeout: 5 * time.Second, Parallel: 4}, each)
	took := time.Since(began)
	for i, m := range got {
		if !m.OK() || m.Delay < 400*time.Millisecond {
			t.Fatalf("#%d: %+v", i, m)
		}
	}
	if took > 3*time.Second {
		t.Fatalf("8 probes of 0.5 s with 4 workers took %v", took)
	}
	if n := procs.allGone(t); n != 4 {
		t.Fatalf("probers spawned = %d, want 4", n)
	}
	if s.Pid() != 0 {
		t.Fatal("measuring must not start the tunnel engine")
	}
	t.Logf("8 probes, 4 workers: %v", took)
}

func TestMeasureReportsEachFailure(t *testing.T) {
	url := target(t) + "/generate_204"
	var procs spawned
	s := New(Options{Command: procs.command("serve")})
	defer s.Close()
	targets := []Target{
		{Config: []byte(direct), Tag: "proxy"},
		{Config: []byte(direct), Tag: "dead"},
		{Config: []byte(`{"outbounds":[{"protocol":"nope"}]}`), Tag: "proxy"},
		{Config: []byte(direct), Tag: "missing"},
		{Config: []byte(direct), Tag: "proxy"},
	}
	got, each := collect(len(targets))
	s.Measure(context.Background(), targets, MeasureOptions{URL: url, Parallel: 2}, each)
	want := []engine.Code{"", engine.CodeProbe, engine.CodeConfig, engine.CodeProbe, ""}
	for i, m := range got {
		if m.Code != want[i] {
			t.Fatalf("#%d: %+v, want code %q", i, m, want[i])
		}
	}
	procs.allGone(t)
}

func TestMeasureSurvivesProberCrash(t *testing.T) {
	url := target(t) + "/generate_204"
	var procs spawned
	s := New(Options{Command: procs.command("crashy")})
	defer s.Close()
	targets := []Target{
		{Config: []byte(direct), Tag: "proxy"},
		{Config: []byte(direct), Tag: "boom"},
		{Config: []byte(direct), Tag: "proxy"},
		{Config: []byte(direct), Tag: "proxy"},
	}
	got, each := collect(len(targets))
	s.Measure(context.Background(), targets, MeasureOptions{URL: url, Parallel: 1}, each)
	want := []engine.Code{"", CodeUnavailable, "", ""}
	for i, m := range got {
		if m.Code != want[i] {
			t.Fatalf("#%d: %+v, want code %q", i, m, want[i])
		}
	}
	if n := procs.allGone(t); n != 3 {
		t.Fatalf("probers spawned = %d, want 3", n)
	}
}

func TestMeasureStopsOnCancel(t *testing.T) {
	base := target(t)
	var procs spawned
	s := New(Options{Command: procs.command("serve")})
	defer s.Close()
	targets := make([]Target, 6)
	for i := range targets {
		targets[i] = Target{Config: []byte(direct), Tag: "proxy"}
	}
	got, each := collect(len(targets))
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(700*time.Millisecond, cancel)
	began := time.Now()
	s.Measure(ctx, targets, MeasureOptions{URL: base + "/slow", Timeout: 15 * time.Second, Parallel: 2}, each)
	if took := time.Since(began); took > 4*time.Second {
		t.Fatalf("measure kept running after cancel: %v", took)
	}
	canceled, untouched := 0, 0
	for _, m := range got {
		switch m.Code {
		case CodeCanceled:
			canceled++
		case "untouched":
			untouched++
		default:
			t.Fatalf("unexpected result %+v", m)
		}
	}
	if canceled < 2 || canceled+untouched != len(targets) {
		t.Fatalf("canceled = %d, untouched = %d", canceled, untouched)
	}
	procs.allGone(t)
}
