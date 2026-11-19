package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
)

const probeConfig = `{"log":{"loglevel":"none"},"outbounds":[{"tag":"proxy","protocol":"freedom"},{"tag":"dead","protocol":"blackhole"}]}`

func probeTarget(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/generate_204":
			w.WriteHeader(http.StatusNoContent)
		case "/slow":
			select {
			case <-r.Context().Done():
			case <-time.After(10 * time.Second):
			}
		case "/moved":
			http.Redirect(w, r, "/generate_204", http.StatusFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func probeCode(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestProbe(t *testing.T) {
	srv := probeTarget(t)
	inst, err := Start([]byte(probeConfig))
	if err != nil {
		t.Fatal(err)
	}
	defer inst.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, tag := range []string{"", "proxy"} {
		took, err := inst.Probe(ctx, srv.URL+"/generate_204", tag)
		if err != nil || took <= 0 {
			t.Fatalf("tag %q: took %v, err %v", tag, took, err)
		}
	}
	for _, c := range []struct{ path, tag string }{
		{"/generate_204", "dead"},
		{"/generate_204", "missing"},
		{"/broken", "proxy"},
		{"/moved", "proxy"},
	} {
		if _, err := inst.Probe(ctx, srv.URL+c.path, c.tag); probeCode(err) != CodeProbe {
			t.Fatalf("%s via %q: err = %v", c.path, c.tag, err)
		}
	}
	if _, err := inst.Probe(ctx, "://", ""); probeCode(err) != CodeConfig {
		t.Fatalf("bad url: err = %v", err)
	}
}

func TestProbeHonorsDeadline(t *testing.T) {
	srv := probeTarget(t)
	inst, err := Start([]byte(probeConfig))
	if err != nil {
		t.Fatal(err)
	}
	defer inst.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	began := time.Now()
	if _, err := inst.Probe(ctx, srv.URL+"/slow", "proxy"); probeCode(err) != CodeProbe {
		t.Fatalf("err = %v", err)
	}
	if took := time.Since(began); took > 2*time.Second {
		t.Fatalf("probe ignored the deadline: %v", took)
	}
}

func TestProbeTakesTheFirstTargetThatAnswers(t *testing.T) {
	srv := probeTarget(t)
	saved := ProbeURLs
	defer func() { ProbeURLs = saved }()
	inst, err := Start([]byte(probeConfig))
	if err != nil {
		t.Fatal(err)
	}
	defer inst.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, urls := range [][]string{
		{srv.URL + "/slow", srv.URL + "/generate_204"},
		{srv.URL + "/generate_204", srv.URL + "/broken"},
		{srv.URL + "/broken", srv.URL + "/generate_204"},
	} {
		ProbeURLs = urls
		began := time.Now()
		if _, err := inst.Probe(ctx, "", "proxy"); err != nil {
			t.Fatalf("%v: %v", urls, err)
		}
		if took := time.Since(began); took > 2*time.Second {
			t.Fatalf("%v: waited for the slow target: %v", urls, took)
		}
	}
	ProbeURLs = []string{srv.URL + "/broken", srv.URL + "/moved"}
	if _, err := inst.Probe(ctx, "", "proxy"); probeCode(err) != CodeProbe {
		t.Fatalf("no target answers: err = %v", err)
	}
	ProbeURLs = []string{srv.URL + "/generate_204", srv.URL + "/generate_204"}
	if _, err := inst.Probe(ctx, "", "dead"); probeCode(err) != CodeProbe {
		t.Fatalf("dead outbound: err = %v", err)
	}
}

func TestOneInstancePerProcess(t *testing.T) {
	srv := probeTarget(t)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := Measure(ctx, []byte(probeConfig), srv.URL+"/generate_204", "proxy"); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if _, err := Measure(context.Background(), []byte(`{"outbounds":[{"protocol":"nope"}]}`), srv.URL, ""); probeCode(err) != CodeConfig {
		t.Fatalf("bad config: err = %v", err)
	}
	inst, err := Start([]byte(probeConfig))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Start([]byte(probeConfig)); probeCode(err) != CodeBusy {
		t.Fatalf("second instance: err = %v", err)
	}
	if _, err := Measure(context.Background(), []byte(probeConfig), srv.URL+"/generate_204", "proxy"); probeCode(err) != CodeBusy {
		t.Fatalf("measure next to a running instance: err = %v", err)
	}
	inst.Close()
	inst.Close()
	again, err := Start([]byte(probeConfig))
	if err != nil {
		t.Fatalf("start after close: %v", err)
	}
	again.Close()
}

func TestProbeModeLoadsEveryFixtureServer(t *testing.T) {
	for i, raw := range fixtureServers(t, "../../testdata/sub/sub.json") {
		cfg, err := xcfg.Build(raw, xcfg.Options{Mode: xcfg.ModeProbe})
		if err != nil {
			t.Fatalf("#%d: %v", i, err)
		}
		if err := Validate(cfg); err != nil {
			t.Fatalf("#%d: %v", i, err)
		}
		if xcfg.MainTag(raw) != "proxy" {
			t.Fatalf("#%d: main tag = %q", i, xcfg.MainTag(raw))
		}
	}
}

func TestServeProbeAndMeasure(t *testing.T) {
	srv := probeTarget(t)
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- Serve(inR, outW) }()
	replies := make(chan Reply, 64)
	go func() {
		lines := bufio.NewScanner(outR)
		lines.Buffer(make([]byte, 64<<10), maxLine)
		for lines.Scan() {
			var m Reply
			if json.Unmarshal(lines.Bytes(), &m) == nil && m.Ev != "log" {
				replies <- m
			}
		}
		close(replies)
	}()
	next := func() Reply {
		t.Helper()
		select {
		case m, ok := <-replies:
			if !ok {
				t.Fatal("stream closed")
			}
			return m
		case <-time.After(10 * time.Second):
			t.Fatal("no reply")
		}
		return Reply{}
	}
	send := func(r Request) {
		b, _ := json.Marshal(r)
		inW.Write(append(b, '\n'))
	}
	if m := next(); m.Ev != "ready" {
		t.Fatalf("first message = %+v", m)
	}
	send(Request{ID: 1, Op: "probe", URL: srv.URL + "/generate_204"})
	if m := next(); m.Re != 1 || m.OK || m.Code != CodeIdle {
		t.Fatalf("probe before start = %+v", m)
	}
	send(Request{ID: 2, Op: "start", Config: json.RawMessage(probeConfig)})
	if m := next(); m.Re != 2 || !m.OK {
		t.Fatalf("start = %+v", m)
	}
	send(Request{ID: 3, Op: "probe", URL: srv.URL + "/generate_204", Tag: "proxy"})
	if m := next(); m.Re != 3 || !m.OK || m.Ms < 1 {
		t.Fatalf("probe = %+v", m)
	}
	send(Request{ID: 4, Op: "probe", URL: srv.URL + "/generate_204", Tag: "dead"})
	if m := next(); m.Re != 4 || m.OK || m.Code != CodeProbe {
		t.Fatalf("probe via dead outbound = %+v", m)
	}
	send(Request{ID: 5, Op: "measure", Config: json.RawMessage(probeConfig), URL: srv.URL + "/generate_204", Tag: "proxy"})
	if m := next(); m.Re != 5 || m.OK || m.Code != CodeBusy {
		t.Fatalf("measure next to a running instance = %+v", m)
	}
	send(Request{ID: 20, Op: "probe", URL: srv.URL + "/slow", Tag: "proxy", Timeout: 30000})
	time.Sleep(100 * time.Millisecond)
	began := time.Now()
	send(Request{ID: 21, Op: "stop"})
	got := map[uint64]Reply{}
	for len(got) < 2 {
		m := next()
		got[m.Re] = m
	}
	if took := time.Since(began); took > 3*time.Second {
		t.Fatalf("stop waited for the probe: %v", took)
	}
	if !got[21].OK || got[20].OK || got[20].Code != CodeProbe {
		t.Fatalf("replies = %+v", got)
	}
	for id := uint64(30); id < 34; id++ {
		send(Request{ID: id, Op: "measure", Config: json.RawMessage(probeConfig), URL: srv.URL + "/generate_204", Tag: "proxy"})
	}
	for id := uint64(30); id < 34; id++ {
		if m := next(); m.Re != id || !m.OK {
			t.Fatalf("measure reply = %+v", m)
		}
	}
	send(Request{ID: 40, Op: "measure", Config: json.RawMessage(probeConfig), URL: srv.URL + "/generate_204", Tag: "dead"})
	if m := next(); m.Re != 40 || m.OK || m.Code != CodeProbe {
		t.Fatalf("measure via dead outbound = %+v", m)
	}
	inW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not exit after input closed")
	}
}
