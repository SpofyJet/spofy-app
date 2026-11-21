package ipc

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/app"
	"github.com/SpofyJet/spofy-app/core/internal/app/apptest"
	"github.com/SpofyJet/spofy-app/core/internal/store"
	"github.com/SpofyJet/spofy-app/core/internal/sub"
	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
)

var rewrite = flag.Bool("update", false, "rewrite proto/samples/session.jsonl")

const samples = "../../../proto/samples/session.jsonl"

type rig struct {
	t        *testing.T
	panel    *apptest.Panel
	tunnel   *apptest.Tunnel
	clock    *apptest.Clock
	app      *app.App
	in       *io.PipeWriter
	lines    chan string
	done     chan error
	next     uint64
	log      []string
	releases *apptest.Releases
	updates  string
}

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func newRig(t *testing.T) *rig {
	r := &rig{t: t, panel: apptest.NewPanel(t), tunnel: apptest.NewTunnel(t), clock: apptest.NewClock(), lines: make(chan string, 256), done: make(chan error, 1)}
	r.tunnel.Answer(map[string]time.Duration{"de": ms(80), "nl": ms(40), "us": ms(120)})
	st, err := store.Open(t.TempDir(), make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	r.releases = apptest.NewReleases(t, r.panel)
	r.updates = filepath.Join(t.TempDir(), "updates")
	r.app = app.New(app.Config{
		Store:   st,
		Tunnel:  r.tunnel,
		Device:  sub.Device{HWID: "hw-1", OS: "Android", OSVersion: "15", Model: "Pixel 8", AppVersion: "0.1.0"},
		HTTP:    r.panel.Client(),
		Run:     xcfg.Options{Mode: xcfg.ModeProbe},
		Now:     r.clock.Now,
		Version: "0.1.0",
		Build:   1,
		Engine:  "xray-test",
		Updates: app.Updates{Sources: []string{apptest.ReleasesURL}, Keys: []ed25519.PublicKey{r.releases.Public()}, Platform: apptest.Platform, Dir: r.updates, Every: -1},
		Retries: []time.Duration{0},
	})
	t.Cleanup(r.app.Close)
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	r.in = inW
	go func() {
		r.done <- Serve(context.Background(), inR, outW, r.app, nil)
		outW.Close()
	}()
	go func() {
		sc := bufio.NewScanner(outR)
		sc.Buffer(make([]byte, 64<<10), maxLine)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
		close(r.lines)
	}()
	t.Cleanup(func() { inW.Close() })
	return r
}

func (r *rig) write(line string) {
	r.log = append(r.log, "> "+line)
	if _, err := r.in.Write([]byte(line + "\n")); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) read() (string, map[string]any) {
	r.t.Helper()
	select {
	case line, ok := <-r.lines:
		if !ok {
			r.t.Fatal("channel closed")
		}
		r.log = append(r.log, "< "+line)
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			r.t.Fatalf("core sent a broken line: %s", line)
		}
		return line, m
	case <-time.After(10 * time.Second):
		r.t.Fatal("no message from the core")
	}
	return "", nil
}

func (r *rig) call(op string, fields string) map[string]any {
	r.t.Helper()
	r.next++
	line := fmt.Sprintf(`{"id":%d,"op":%q`, r.next, op)
	if fields != "" {
		line += "," + fields
	}
	r.write(line + "}")
	for {
		_, m := r.read()
		if re, ok := m["re"].(float64); ok && uint64(re) == r.next {
			return m
		}
	}
}

func (r *rig) expect(m map[string]any, code string) {
	r.t.Helper()
	if code == "" && m["ok"] != true {
		r.t.Fatalf("reply = %v", m)
	}
	if code != "" && (m["ok"] != false || m["code"] != code) {
		r.t.Fatalf("reply = %v, want code %s", m, code)
	}
}

func TestSessionTranscript(t *testing.T) {
	r := newRig(t)
	step := func(op, fields, code string) map[string]any {
		t.Helper()
		r.clock.Advance(time.Minute)
		m := r.call(op, fields)
		r.expect(m, code)
		return m
	}
	step("hello", "", "")
	step("connect", "", "no_subscription")
	step("subscribe", `"url":"http://sub.example.test/x"`, "sub_bad_url")
	step("subscribe", fmt.Sprintf(`"url":%q`, apptest.Address), "")
	step("measure", "", "")
	step("connect", `"server":"auto"`, "")
	r.tunnel.Count(2048, 1048576)
	step("traffic", "", "")
	step("set", `"key":"killSwitch","value":true`, "")
	step("set", `"key":"apps","value":{"mode":"except","list":["org.example.one","org.example.two"]}`, "")
	r.tunnel.Kill("nl")
	step("check", "", "")
	r.tunnel.Kill("nl", "de")
	step("online", `"online":false`, "")
	step("check", "", "probe_failed")
	step("online", `"online":true`, "")
	step("connect", `"server":"de"`, "server_unreachable")
	step("connect", `"server":"us"`, "")
	step("connect", `"server":"mars"`, "server_not_found")
	step("measure", "", "connected")
	step("disconnect", "", "")
	step("report", "", "")
	step("logs", "", "")
	step("clearLogs", "", "")
	step("logs", "", "")
	step("refresh", "", "")
	step("set", `"key":"apps","value":null`, "")
	step("set", `"key":"probeTimeout","value":3`, "")
	step("set", `"key":"dns","value":"10.0.0.1"`, "bad_value")
	first := r.app.Snapshot().List[0].ID
	step("subscribe", `"url":"https://sub.example.test/sub/SECOND0123456789"`, "")
	step("activate", fmt.Sprintf(`"subscription":%q`, first), "")
	second := r.app.Snapshot().List[1].ID
	step("activate", `"subscription":"000000000000"`, "sub_unknown")
	step("hello", "", "")
	step("teleport", "", "unknown_op")
	step("unsubscribe", fmt.Sprintf(`"subscription":%q`, second), "")
	step("unsubscribe", "", "")
	step("hello", "", "")
	step("checkUpdate", "", "update_unreachable")
	r.releases.Publish(2, 100, []byte("spofy update"), []byte("spofy update"))
	step("checkUpdate", "", "")
	step("fetchUpdate", "", "")
	step("installUpdate", "", "update_off")
	got := strings.ReplaceAll(strings.Join(r.log, "\n")+"\n", r.updates, "/state/updates")
	if strings.Contains(got, "TESTTOKEN") && strings.Count(got, "TESTTOKEN") != 1 {
		t.Fatal("the subscription address appears outside the subscribe request")
	}
	if *rewrite {
		if err := os.MkdirAll(filepath.Dir(samples), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(samples, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(samples)
	if err != nil {
		t.Fatalf("%v (run the test with -update to create the samples)", err)
	}
	if got != string(want) {
		a, b := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := 0; i < len(a) && i < len(b); i++ {
			if a[i] != b[i] {
				t.Fatalf("line %d differs from proto/samples/session.jsonl\n got: %.300s\nwant: %.300s", i+1, a[i], b[i])
			}
		}
		t.Fatalf("transcript has %d lines, samples have %d", len(a), len(b))
	}
}

func TestSamplesDecodeStrictly(t *testing.T) {
	data, err := os.ReadFile(samples)
	if err != nil {
		t.Skip("no samples yet")
	}
	requests := 0
	for n, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		body, ok := strings.CutPrefix(line, "> ")
		if !ok {
			if !strings.HasPrefix(line, "< ") || !json.Valid([]byte(line[2:])) {
				t.Fatalf("line %d is not a message", n+1)
			}
			continue
		}
		requests++
		var req Request
		dec := json.NewDecoder(strings.NewReader(body))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			t.Fatalf("line %d: %v", n+1, err)
		}
		again, _ := json.Marshal(req)
		var a, b any
		json.Unmarshal([]byte(body), &a)
		json.Unmarshal(again, &b)
		x, _ := json.Marshal(a)
		y, _ := json.Marshal(b)
		if !bytes.Equal(x, y) {
			t.Fatalf("line %d does not survive a round trip: %s -> %s", n+1, body, again)
		}
	}
	if requests < 20 {
		t.Fatalf("only %d requests in the samples", requests)
	}
}

func TestBadLines(t *testing.T) {
	r := newRig(t)
	for _, line := range []string{`not json`, `{"id":1}`, `{"op":"hello"}`, `{"id":2,"op":"hello","extra":1}`, `{"id":3,"op":"online"}`, `{"id":4,"op":"activate"}`, `[1,2]`} {
		r.write(line)
		if _, m := r.read(); m["ok"] != false || m["code"] != "bad_request" {
			t.Fatalf("%s -> %v", line, m)
		}
	}
	r.write("")
	r.write("   ")
	if m := r.call("hello", ""); m["ok"] != true {
		t.Fatalf("the channel did not survive bad lines: %v", m)
	}
}

func TestDisconnectInterruptsConnect(t *testing.T) {
	r := newRig(t)
	r.expect(r.call("subscribe", fmt.Sprintf(`"url":%q`, apptest.Address)), "")
	release := r.tunnel.Hold()
	defer release()
	r.write(`{"id":50,"op":"connect","server":"de"}`)
	for {
		if line, _ := r.read(); strings.Contains(line, `"state":"connecting"`) {
			break
		}
	}
	began := time.Now()
	r.write(`{"id":51,"op":"disconnect"}`)
	replies := map[float64]map[string]any{}
	for len(replies) < 2 {
		if _, m := r.read(); m["re"] != nil {
			replies[m["re"].(float64)] = m
		}
	}
	if took := time.Since(began); took > 2*time.Second {
		t.Fatalf("disconnect waited %v", took)
	}
	if replies[50]["code"] != "canceled" || replies[51]["ok"] != true || r.tunnel.Running() != "" {
		t.Fatalf("replies = %v, running %q", replies, r.tunnel.Running())
	}
	if m := r.call("hello", ""); m["snapshot"].(map[string]any)["connection"].(map[string]any)["state"] != "idle" {
		t.Fatalf("snapshot = %v", m)
	}
}

func TestClosedInputStopsPendingWork(t *testing.T) {
	r := newRig(t)
	release := r.panel.Hold()
	defer release()
	r.write(fmt.Sprintf(`{"id":1,"op":"subscribe","url":%q}`, apptest.Address))
	for r.panel.Hits() == 0 {
		time.Sleep(time.Millisecond)
	}
	began := time.Now()
	r.in.Close()
	select {
	case err := <-r.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return after the input closed")
	}
	if took := time.Since(began); took > 2*time.Second {
		t.Fatalf("serve waited %v for the pending request", took)
	}
	if _, m := r.read(); m["code"] != "sub_unreachable" {
		t.Fatalf("pending request got %v", m)
	}
}
