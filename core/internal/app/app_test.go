package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/app/apptest"
	"github.com/SpofyJet/spofy-app/core/internal/engine"
	"github.com/SpofyJet/spofy-app/core/internal/session"
	"github.com/SpofyJet/spofy-app/core/internal/store"
	"github.com/SpofyJet/spofy-app/core/internal/sub"
	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
)

type bench struct {
	t      *testing.T
	dir    string
	panel  *apptest.Panel
	tunnel *apptest.Tunnel
	clock  *apptest.Clock
	app    *App

	mu     sync.Mutex
	events []Event
}

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func newBench(t *testing.T, tune func(*Config)) *bench {
	b := &bench{t: t, dir: t.TempDir(), panel: apptest.NewPanel(t), tunnel: apptest.NewTunnel(t), clock: apptest.NewClock()}
	b.tunnel.Answer(map[string]time.Duration{"de": ms(80), "nl": ms(40), "us": ms(120), "обход1": ms(10)})
	b.open(tune)
	return b
}

func (b *bench) open(tune func(*Config)) {
	st, err := store.Open(b.dir, make([]byte, 32))
	if err != nil {
		b.t.Fatal(err)
	}
	cfg := Config{
		Store:   st,
		Tunnel:  b.tunnel,
		Device:  sub.Device{HWID: "hw-1", OS: "Android", OSVersion: "15", Model: "Pixel 8", AppVersion: "0.1.0"},
		HTTP:    b.panel.Client(),
		Run:     xcfg.Options{Mode: xcfg.ModeProxy, Proxy: xcfg.Proxy{Port: 2080, User: "u", Pass: "p"}},
		Now:     b.clock.Now,
		Version: "0.1.0",
		Retries: []time.Duration{0},
	}
	if tune != nil {
		tune(&cfg)
	}
	b.app = New(cfg)
	b.app.Listen(func(e Event) {
		b.mu.Lock()
		b.events = append(b.events, e)
		b.mu.Unlock()
	})
	b.t.Cleanup(b.app.Close)
}

func (b *bench) seen() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, e := range b.events {
		s := e.Ev
		if e.Ev == EvConnection {
			s = string(e.Connection.State)
			if e.Connection.Server != "" {
				s += ":" + e.Connection.Server
			}
		}
		out = append(out, s)
	}
	b.events = nil
	return out
}

func (b *bench) subscribed() *bench {
	b.t.Helper()
	if err := b.app.Subscribe(context.Background(), apptest.Address); err != nil {
		b.t.Fatal(err)
	}
	b.seen()
	return b
}

func code(err error) engine.Code {
	var e *engine.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func same(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func find(list []Server, id string) Server {
	for _, s := range list {
		if s.ID == id {
			return s
		}
	}
	return Server{}
}

func TestSubscribe(t *testing.T) {
	b := newBench(t, nil)
	if snap := b.app.Snapshot(); snap.Subscription != nil || len(snap.Servers) != 0 || snap.Connection.State != session.Idle || snap.Connection.Choice != Auto || snap.Protocol != Protocol {
		t.Fatalf("fresh snapshot = %+v", snap)
	}
	if err := b.app.Subscribe(context.Background(), "  "+apptest.Address+"\n"); err != nil {
		t.Fatal(err)
	}
	same(t, b.seen(), []string{EvSubscription, EvServers})
	snap := b.app.Snapshot()
	s := snap.Subscription
	if len(snap.Servers) != 19 || s == nil || s.Total != 107374182400 || s.Download != 452716590 || s.Expire != 1798761600 || s.UpdateEvery != 3600 || s.FetchedAt != b.clock.Now().Unix() {
		t.Fatalf("snapshot = %+v, subscription = %+v", snap, s)
	}
	if de := find(snap.Servers, "de"); de.Country != "DE" || de.Protocol == "" || de.DelayMs != 0 {
		t.Fatalf("server = %+v", de)
	}
	if h := b.panel.Last(); h.Get("X-Hwid") != "hw-1" || !strings.HasPrefix(h.Get("User-Agent"), "Spofy/0.1.0") {
		t.Fatalf("request headers = %v", h)
	}
	raw, _ := json.Marshal(snap)
	if strings.Contains(string(raw), "TESTTOKEN") {
		t.Fatal("the subscription address leaked into the snapshot")
	}
	b.open(nil)
	if again := b.app.Snapshot(); len(again.Servers) != 19 || again.Subscription.Title != s.Title {
		t.Fatalf("after restart: %+v", again)
	}
}

func TestSubscribeErrors(t *testing.T) {
	b := newBench(t, nil)
	for _, address := range []string{"", "http://sub.example.test/x", "sub.example.test/x", "spofy://x", "https://"} {
		if err := b.app.Subscribe(context.Background(), address); code(err) != CodeBadURL {
			t.Fatalf("%q: err = %v", address, err)
		}
	}
	if b.panel.Hits() != 0 {
		t.Fatal("a bad address reached the network")
	}
	b.panel.Status(http.StatusForbidden)
	if err := b.app.Subscribe(context.Background(), apptest.Address); code(err) != CodeStatus || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v", err)
	}
	b.panel.Status(0)
	b.panel.Set(func(body *[]byte, _ http.Header) { *body = []byte("<html>") })
	if err := b.app.Subscribe(context.Background(), apptest.Address); code(err) != CodeFormat {
		t.Fatalf("err = %v", err)
	}
	b.panel.Close()
	err := b.app.Subscribe(context.Background(), apptest.Address)
	if code(err) != CodeUnreachable || strings.Contains(err.Error(), "TESTTOKEN") || strings.Contains(err.Error(), "/sub/") {
		t.Fatalf("err = %v", err)
	}
	if snap := b.app.Snapshot(); snap.Subscription != nil || len(b.seen()) != 0 {
		t.Fatalf("a failed subscribe changed the state: %+v", snap)
	}
}

func TestRefresh(t *testing.T) {
	b := newBench(t, nil)
	if _, err := b.app.Refresh(context.Background()); code(err) != CodeNoSubscription {
		t.Fatalf("err = %v", err)
	}
	b.subscribed()
	b.clock.Advance(time.Hour)
	b.panel.Usage("upload=5; download=7; total=100; expire=1798761600")
	changed, err := b.app.Refresh(context.Background())
	if err != nil || changed {
		t.Fatalf("changed %v, err %v", changed, err)
	}
	same(t, b.seen(), []string{EvSubscription})
	if h := b.panel.Last(); h.Get("If-None-Match") != `W/"fixture-1"` {
		t.Fatalf("request headers = %v", h)
	}
	if s := b.app.Snapshot().Subscription; s.FetchedAt != b.clock.Now().Unix() || s.Download != 7 || s.Total != 100 {
		t.Fatalf("subscription = %+v", s)
	}
	if err := b.app.Connect("nl"); err != nil {
		t.Fatal(err)
	}
	if err := b.app.Disconnect(); err != nil {
		t.Fatal(err)
	}
	b.app.Measure(context.Background())
	b.seen()
	b.panel.Set(func(body *[]byte, h http.Header) {
		var list []json.RawMessage
		json.Unmarshal(*body, &list)
		*body, _ = json.Marshal(append(list[:4:4], list[5:]...))
		h.Set("Etag", `W/"fixture-2"`)
	})
	changed, err = b.app.Refresh(context.Background())
	if err != nil || !changed {
		t.Fatalf("changed %v, err %v", changed, err)
	}
	same(t, b.seen(), []string{"idle", EvSubscription, EvServers})
	snap := b.app.Snapshot()
	if len(snap.Servers) != 18 || find(snap.Servers, "nl").ID != "" || snap.Connection.Choice != Auto || find(snap.Servers, "de").DelayMs != 80 {
		t.Fatalf("snapshot = %+v", snap)
	}
	b.panel.Status(http.StatusBadGateway)
	if _, err := b.app.Refresh(context.Background()); code(err) != CodeStatus {
		t.Fatalf("err = %v", err)
	}
	if len(b.app.Snapshot().Servers) != 18 {
		t.Fatal("a failed refresh dropped the servers")
	}
}

func TestConnectAuto(t *testing.T) {
	b := newBench(t, nil).subscribed()
	if err := b.app.Connect(Auto); err != nil {
		t.Fatal(err)
	}
	same(t, b.seen(), []string{"measuring", EvServers, "connecting:обход1", "connected:обход1"})
	snap := b.app.Snapshot()
	if c := snap.Connection; c.State != session.Connected || c.Server != "обход1" || c.DelayMs != 10 || c.Choice != Auto {
		t.Fatalf("connection = %+v", c)
	}
	if nl := find(snap.Servers, "nl"); nl.DelayMs != 40 || nl.MeasuredAt != b.clock.Now().Unix() || find(snap.Servers, "sg").DelayMs != 0 {
		t.Fatalf("servers = %+v", snap.Servers)
	}
	if err := b.app.Disconnect(); err != nil {
		t.Fatal(err)
	}
	b.clock.Advance(time.Hour)
	if err := b.app.Connect(""); err != nil {
		t.Fatal(err)
	}
	same(t, b.seen(), []string{"idle", "connecting:обход1", "connected:обход1"})
	if b.tunnel.Measured() != 1 {
		t.Fatalf("measured %d times", b.tunnel.Measured())
	}
	b.app.Disconnect()
	b.clock.Advance(6 * time.Hour)
	if err := b.app.Connect(""); err != nil || b.tunnel.Measured() != 2 {
		t.Fatalf("an old measurement was trusted: err %v, measured %d", err, b.tunnel.Measured())
	}
}

func TestReserveTier(t *testing.T) {
	reserve := func(c *Config) {
		c.Tier = func(s sub.Server) int {
			if strings.HasPrefix(s.ID, "обход") {
				return 1
			}
			return 0
		}
	}
	b := newBench(t, reserve).subscribed()
	if err := b.app.Connect(Auto); err != nil || b.tunnel.Running() != "nl" {
		t.Fatalf("err %v, running %q", err, b.tunnel.Running())
	}
	if s := find(b.app.Snapshot().Servers, "обход1"); s.Tier != 1 {
		t.Fatalf("server = %+v", s)
	}
	b.tunnel.Kill("nl", "de", "us")
	if err := b.app.Check(); err != nil || b.tunnel.Running() != "обход1" {
		t.Fatalf("err %v, running %q", err, b.tunnel.Running())
	}
}

func TestConnectPinned(t *testing.T) {
	b := newBench(t, nil)
	if err := b.app.Connect("de"); code(err) != CodeNoSubscription {
		t.Fatalf("err = %v", err)
	}
	b.subscribed()
	if err := b.app.Connect("xx"); code(err) != session.CodeNotFound {
		t.Fatalf("err = %v", err)
	}
	if err := b.app.Connect("us"); err != nil {
		t.Fatal(err)
	}
	same(t, b.seen(), []string{"idle", "connecting:us", "connected:us"})
	b.tunnel.Kill("us")
	if err := b.app.Check(); code(err) != session.CodeUnreachable || b.tunnel.Running() != "us" {
		t.Fatalf("err %v, running %q", err, b.tunnel.Running())
	}
	same(t, b.seen(), []string{"stalled:us"})
	if err := b.app.Connect("sg"); code(err) != session.CodeUnreachable {
		t.Fatalf("err = %v", err)
	}
	b.open(nil)
	if c := b.app.Snapshot().Connection; c.Choice != "sg" || c.State != session.Idle {
		t.Fatalf("after restart: %+v", c)
	}
}

func TestCheckSwitchesAndOnline(t *testing.T) {
	b := newBench(t, nil).subscribed()
	if err := b.app.Connect(Auto); err != nil {
		t.Fatal(err)
	}
	b.seen()
	b.tunnel.Kill("обход1")
	if err := b.app.SetOnline(false); err != nil {
		t.Fatal(err)
	}
	if err := b.app.Check(); err == nil {
		t.Fatal("check passed on a dead server")
	}
	same(t, b.seen(), []string{"offline:обход1"})
	if err := b.app.SetOnline(true); err != nil || b.tunnel.Running() != "nl" {
		t.Fatalf("err %v, running %q", err, b.tunnel.Running())
	}
	same(t, b.seen(), []string{"switching:nl", "connected:nl"})
}

func TestMeasure(t *testing.T) {
	b := newBench(t, nil)
	if err := b.app.Measure(context.Background()); code(err) != CodeNoSubscription {
		t.Fatalf("err = %v", err)
	}
	b.subscribed()
	if err := b.app.Measure(context.Background()); err != nil {
		t.Fatal(err)
	}
	same(t, b.seen(), []string{EvServers})
	if s := b.app.Snapshot().Servers; find(s, "us").DelayMs != 120 || find(s, "by").DelayMs != 0 {
		t.Fatalf("servers = %+v", s)
	}
	b.tunnel.Kill("us")
	b.app.Measure(context.Background())
	if s := find(b.app.Snapshot().Servers, "us"); s.DelayMs != 0 || s.MeasuredAt != 0 {
		t.Fatalf("a silent server kept its old delay: %+v", s)
	}
	if err := b.app.Connect("de"); err != nil {
		t.Fatal(err)
	}
	if err := b.app.Measure(context.Background()); code(err) != CodeConnected {
		t.Fatalf("measure through the tunnel: err = %v", err)
	}
}

func TestKillSwitchSetting(t *testing.T) {
	b := newBench(t, nil).subscribed()
	b.tunnel.Answer(nil)
	if err := b.app.Connect(Auto); code(err) != session.CodeNoServer || b.tunnel.Running() != "" || b.app.Snapshot().Connection.State != session.Failed {
		t.Fatalf("switch off: err %v, running %q, state %s", err, b.tunnel.Running(), b.app.Snapshot().Connection.State)
	}
	for _, key := range []string{KeyKillSwitch, KeyAutoConnect} {
		for _, bad := range []string{`"yes"`, `1`, `[true]`, `{}`} {
			if err := b.app.Set(key, json.RawMessage(bad)); code(err) != CodeValue {
				t.Fatalf("%s = %s: err %v", key, bad, err)
			}
		}
	}
	if err := b.app.Set(KeyKillSwitch, json.RawMessage(`true`)); err != nil {
		t.Fatal(err)
	}
	stalls := func(when string) {
		t.Helper()
		if err := b.app.Connect(Auto); code(err) != session.CodeNoServer || b.tunnel.Running() == "" || b.app.Snapshot().Connection.State != session.Stalled {
			t.Fatalf("%s: err %v, running %q, state %s", when, err, b.tunnel.Running(), b.app.Snapshot().Connection.State)
		}
		if err := b.app.Disconnect(); err != nil || b.tunnel.Running() != "" {
			t.Fatalf("%s: disconnect: err %v, running %q", when, err, b.tunnel.Running())
		}
	}
	stalls("switch on")
	b.open(nil)
	stalls("after restart")
	if err := b.app.Set(KeyKillSwitch, json.RawMessage(`false`)); err != nil {
		t.Fatal(err)
	}
	if err := b.app.Connect(Auto); code(err) != session.CodeNoServer || b.tunnel.Running() != "" {
		t.Fatalf("switch off again: err %v, running %q", err, b.tunnel.Running())
	}
}

func TestResume(t *testing.T) {
	b := newBench(t, nil)
	if err := b.app.Set(KeyAutoConnect, json.RawMessage(`true`)); err != nil {
		t.Fatal(err)
	}
	if err := b.app.Resume(); err != nil || len(b.tunnel.Starts()) != 0 {
		t.Fatalf("no subscription: err %v, starts %v", err, b.tunnel.Starts())
	}
	b.subscribed()
	b.open(nil)
	if err := b.app.Resume(); err != nil || b.tunnel.Running() != "обход1" || b.app.Snapshot().Connection.State != session.Connected {
		t.Fatalf("switch on: err %v, running %q", err, b.tunnel.Running())
	}
	if err := b.app.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if err := b.app.Set(KeyAutoConnect, json.RawMessage(`false`)); err != nil {
		t.Fatal(err)
	}
	b.open(nil)
	if err := b.app.Resume(); err != nil || b.tunnel.Running() != "" {
		t.Fatalf("switch off: err %v, running %q", err, b.tunnel.Running())
	}
}

func TestSettings(t *testing.T) {
	b := newBench(t, nil)
	for _, bad := range []struct {
		key, value string
	}{{"", "1"}, {"k", "{"}, {strings.Repeat("k", 65), "1"}, {"k", `"` + strings.Repeat("v", 5000) + `"`}} {
		if err := b.app.Set(bad.key, json.RawMessage(bad.value)); code(err) != CodeValue {
			t.Fatalf("%q: err = %v", bad.key, err)
		}
	}
	if err := b.app.Set("killSwitch", json.RawMessage(`true`)); err != nil {
		t.Fatal(err)
	}
	if err := b.app.Set("apps", json.RawMessage(`["a.b","c.d"]`)); err != nil {
		t.Fatal(err)
	}
	same(t, b.seen(), []string{EvSettings, EvSettings})
	b.open(nil)
	if s := b.app.Snapshot().Settings; string(s["killSwitch"]) != "true" || string(s["apps"]) != `["a.b","c.d"]` {
		t.Fatalf("settings = %v", s)
	}
	if err := b.app.Set("apps", json.RawMessage(`null`)); err != nil {
		t.Fatal(err)
	}
	if s := b.app.Snapshot().Settings; len(s) != 1 {
		t.Fatalf("settings = %v", s)
	}
}

func TestUnsubscribe(t *testing.T) {
	b := newBench(t, nil).subscribed()
	if err := b.app.Connect("de"); err != nil {
		t.Fatal(err)
	}
	b.seen()
	if err := b.app.Unsubscribe(); err != nil {
		t.Fatal(err)
	}
	same(t, b.seen(), []string{"idle", "idle", EvSubscription, EvServers})
	snap := b.app.Snapshot()
	if snap.Subscription != nil || len(snap.Servers) != 0 || snap.Connection.Choice != Auto || b.tunnel.Running() != "" {
		t.Fatalf("snapshot = %+v, running %q", snap, b.tunnel.Running())
	}
	b.open(nil)
	if snap := b.app.Snapshot(); snap.Subscription != nil {
		t.Fatal("the subscription came back after restart")
	}
}

func TestTrafficAndEvents(t *testing.T) {
	b := newBench(t, nil)
	b.tunnel.Count(10, 20)
	if tr := b.app.Traffic(); tr.Up != 10 || tr.Down != 20 {
		t.Fatalf("traffic = %+v", tr)
	}
	for ev, want := range map[string]string{
		EvConnection:   `{"ev":"connection","connection":{"state":"idle","choice":"auto","server":"","delayMs":0,"code":"","error":""}}`,
		EvSubscription: `{"ev":"subscription","subscription":null}`,
		EvServers:      `{"ev":"servers","servers":[]}`,
		EvSettings:     `{"ev":"settings","settings":{}}`,
	} {
		e := Event{Ev: ev, Connection: Connection{State: session.Idle, Choice: Auto}, Servers: []Server{}, Settings: map[string]json.RawMessage{}}
		if got, _ := json.Marshal(e); string(got) != want {
			t.Fatalf("%s: %s", ev, got)
		}
	}
	heard := 0
	stop := b.app.Listen(func(Event) { heard++ })
	b.app.Set("a", json.RawMessage(`1`))
	stop()
	b.app.Set("a", json.RawMessage(`2`))
	if heard != 1 {
		t.Fatalf("heard %d events", heard)
	}
}
