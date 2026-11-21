package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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
		Store:    st,
		Tunnel:   b.tunnel,
		Device:   sub.Device{HWID: "hw-1", OS: "Android", OSVersion: "15", Model: "Pixel 8", AppVersion: "0.1.0"},
		HTTP:     b.panel.Client(),
		Run:      xcfg.Options{Mode: xcfg.ModeProbe},
		Now:      b.clock.Now,
		Version:  "0.1.0",
		Retries:  []time.Duration{0},
		Renew:    -1,
		Resample: -1,
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

func (b *bench) heard(ev string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range b.events {
		if e.Ev == ev {
			return true
		}
	}
	return false
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
	same(t, b.seen(), []string{EvSubscription, EvServers, EvList})
	snap := b.app.Snapshot()
	s := snap.Subscription
	if len(snap.List) != 1 || !snap.List[0].Active || snap.List[0].Kind != KindURL || snap.List[0].Servers != 19 || snap.List[0].Title != s.Title || len(snap.List[0].ID) != 12 {
		t.Fatalf("list = %+v", snap.List)
	}
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

func renewing(c *Config) { c.Renew = time.Minute }

func (b *bench) until(what string, ok func() bool) {
	b.t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(2 * time.Millisecond) {
		if ok() {
			return
		}
	}
	b.t.Fatalf("did not happen: %s", what)
}

func (b *bench) list(etag string, keep func(remarks string, config map[string]json.RawMessage) bool) {
	full, _ := apptest.Fixture(b.t)
	b.panel.Set(func(body *[]byte, h http.Header) {
		var all, out []map[string]json.RawMessage
		json.Unmarshal(full, &all)
		for _, c := range all {
			var remarks string
			json.Unmarshal(c["remarks"], &remarks)
			if keep == nil || keep(remarks, c) {
				out = append(out, c)
			}
		}
		*body, _ = json.Marshal(out)
		h.Set("Etag", etag)
	})
}

func TestSubscriptionIsFetchedAgainByThePanelIntervalAndNotMoreOften(t *testing.T) {
	b := newBench(t, renewing).subscribed()
	step := func(wait time.Duration, hits int, what string) {
		t.Helper()
		b.clock.Advance(wait)
		b.app.freshen(false)
		if got := b.panel.Hits(); got != hits {
			t.Fatalf("%s: the panel was asked %d times, want %d", what, got, hits)
		}
	}
	step(0, 1, "just fetched")
	step(59*time.Minute, 1, "a minute before the interval")
	step(time.Minute, 2, "the interval has passed")
	if h := b.panel.Last(); h.Get("If-None-Match") != `W/"fixture-1"` {
		t.Fatalf("the scheduled request does not say what is already here: %v", h)
	}
	if s := b.app.Snapshot().Subscription; s.FetchedAt != b.clock.Now().Unix() {
		t.Fatalf("subscription = %+v", s)
	}
	same(t, b.seen(), []string{EvSubscription})
	step(0, 2, "fetched a moment ago")
	b.panel.Status(http.StatusBadGateway)
	step(time.Hour, 3, "the next interval")
	step(14*time.Minute, 3, "the panel failed 14 minutes ago")
	step(time.Minute, 4, "a quarter of the interval after a failure")
	b.panel.Status(0)
	b.app.SetOnline(false)
	step(time.Hour, 4, "no network")
	b.app.SetOnline(true)
	b.until("a fetch when the network is back", func() bool { return b.panel.Hits() == 5 })
	b.until("the fetch is over", func() bool {
		b.app.mu.Lock()
		busy := b.app.renews
		b.app.mu.Unlock()
		return !busy && b.app.Snapshot().Subscription.FetchedAt == b.clock.Now().Unix()
	})
	b.panel.Set(func(_ *[]byte, h http.Header) {
		h.Del("Profile-Update-Interval")
		h.Set("Etag", `W/"fixture-2"`)
	})
	step(time.Hour, 6, "the last interval the panel named")
	step(12*time.Hour-time.Minute, 6, "no interval from the panel, a minute before 12 hours")
	step(time.Minute, 7, "no interval from the panel, 12 hours")
	if len(b.tunnel.Starts()) != 0 {
		t.Fatal("a fetch started the tunnel")
	}
}

func TestIntervalFromThePanelIsNotShorterThanTheFloorAndOffMeansOff(t *testing.T) {
	b := newBench(t, func(c *Config) { c.Renew = 3 * time.Hour }).subscribed()
	b.clock.Advance(3*time.Hour - time.Second)
	b.app.freshen(false)
	if b.panel.Hits() != 1 {
		t.Fatal("fetched before the floor")
	}
	b.clock.Advance(time.Second)
	b.app.freshen(false)
	if b.panel.Hits() != 2 {
		t.Fatal("not fetched at the floor")
	}
	off := newBench(t, nil).subscribed()
	off.clock.Advance(100 * time.Hour)
	off.app.freshen(false)
	off.app.freshen(true)
	if off.panel.Hits() != 1 {
		t.Fatal("fetched though turned off")
	}
}

func TestEventsThatStartAFetch(t *testing.T) {
	b := newBench(t, func(c *Config) {
		c.Renew = time.Minute
		c.Watch = 5 * time.Millisecond
	}).subscribed()
	b.clock.Advance(time.Hour)
	if err := b.app.Connect("nl"); err != nil {
		t.Fatal(err)
	}
	b.until("a fetch after the connection is up", func() bool { return b.panel.Hits() == 2 })
	b.until("the fetch is over", func() bool {
		b.app.mu.Lock()
		busy := b.app.renews
		b.app.mu.Unlock()
		return !busy && b.app.Snapshot().Subscription.FetchedAt == b.clock.Now().Unix()
	})
	b.clock.Advance(time.Hour)
	b.until("a fetch while connected, with nobody asking", func() bool { return b.panel.Hits() == 3 })
	b.until("the fetch is over", func() bool {
		b.app.mu.Lock()
		busy := b.app.renews
		b.app.mu.Unlock()
		return !busy && b.app.Snapshot().Subscription.FetchedAt == b.clock.Now().Unix()
	})
	if err := b.app.Disconnect(); err != nil {
		t.Fatal(err)
	}
	b.clock.Advance(time.Hour)
	time.Sleep(30 * time.Millisecond)
	if b.panel.Hits() != 3 {
		t.Fatal("fetched while disconnected with nobody looking")
	}
	b.app.Check()
	b.until("a fetch when the person looks at the app", func() bool { return b.panel.Hits() == 4 })
	if got := b.tunnel.Starts(); !reflect.DeepEqual(got, []string{"nl"}) {
		t.Fatalf("starts = %v", got)
	}
}

func TestRunningConnectionFollowsTheNewList(t *testing.T) {
	b := newBench(t, renewing).subscribed()
	if err := b.app.Connect(Auto); err != nil {
		t.Fatal(err)
	}
	b.seen()
	refresh := func() {
		t.Helper()
		if changed, err := b.app.Refresh(context.Background()); err != nil || !changed {
			t.Fatalf("changed %v, err %v", changed, err)
		}
	}
	b.list(`W/"same"`, nil)
	refresh()
	b.list(`W/"without-nl"`, func(remarks string, _ map[string]json.RawMessage) bool { return !strings.Contains(remarks, "(NL)") })
	refresh()
	time.Sleep(30 * time.Millisecond)
	same(t, b.seen(), []string{EvSubscription, EvServers, EvSubscription, EvServers})
	if got := b.tunnel.Starts(); !reflect.DeepEqual(got, []string{"обход1"}) {
		t.Fatalf("a list with the same server in use restarted it: %v", got)
	}
	b.tunnel.Kill("обход1")
	if err := b.app.Check(); err != nil || b.tunnel.Running() != "de" {
		t.Fatalf("after the fastest of the old list was removed: err %v, running %q", err, b.tunnel.Running())
	}
	b.seen()
	touched := func(mark string) func(string, map[string]json.RawMessage) bool {
		return func(remarks string, c map[string]json.RawMessage) bool {
			if strings.Contains(remarks, mark) {
				c["routing"] = json.RawMessage(`{"domainStrategy":"AsIs","rules":[]}`)
			}
			return true
		}
	}
	b.list(`W/"de-changed"`, touched("(DE)"))
	refresh()
	b.until("the connection is made again when the settings of its server changed", func() bool {
		return len(b.tunnel.Starts()) > 2 && b.app.Snapshot().Connection.State == session.Connected
	})
	b.tunnel.Kill()
	if err := b.app.Connect("us"); err != nil {
		t.Fatal(err)
	}
	before := len(b.tunnel.Starts())
	b.list(`W/"us-changed"`, touched("(US)"))
	refresh()
	b.until("a chosen server is started again with its new settings", func() bool {
		starts := b.tunnel.Starts()
		c := b.app.Snapshot().Connection
		return len(starts) == before+1 && starts[before] == "us" && c.State == session.Connected && c.Choice == "us"
	})
	b.list(`W/"without-us"`, func(remarks string, _ map[string]json.RawMessage) bool { return !strings.Contains(remarks, "(US)") })
	refresh()
	b.until("a chosen server that is gone gives way to the automatic choice", func() bool {
		c := b.app.Snapshot().Connection
		return c.State == session.Connected && c.Choice == Auto && c.Server != "us" && b.tunnel.Running() == c.Server
	})
}

func TestFailedConnectFetchesTheListAndTriesAgainWhenItIsNew(t *testing.T) {
	b := newBench(t, renewing)
	b.list(`W/"fixture-1"`, func(remarks string, _ map[string]json.RawMessage) bool { return strings.Contains(remarks, "(SG)") })
	b.subscribed()
	if err := b.app.Connect(Auto); code(err) != session.CodeNoServer {
		t.Fatalf("err = %v", err)
	}
	b.until("a fetch after the failure", func() bool { return b.panel.Hits() == 2 })
	time.Sleep(30 * time.Millisecond)
	if c := b.app.Snapshot().Connection; c.State != session.Failed || len(b.tunnel.Starts()) != 0 {
		t.Fatalf("the same list was tried again: %+v", c)
	}
	b.app.freshen(true)
	if b.panel.Hits() != 2 {
		t.Fatal("a second fetch within a minute")
	}
	b.clock.Advance(time.Minute)
	b.list(`W/"whole"`, nil)
	if err := b.app.Connect(Auto); code(err) != session.CodeNoServer {
		t.Fatalf("err = %v", err)
	}
	b.until("the connection comes up by itself with the new list", func() bool {
		c := b.app.Snapshot().Connection
		return c.State == session.Connected && c.Server == "обход1"
	})
	if b.panel.Hits() != 3 {
		t.Fatalf("the panel was asked %d times", b.panel.Hits())
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

func TestOneWayTrafficMovesToAnotherServerWithoutBeingAsked(t *testing.T) {
	b := newBench(t, func(c *Config) { c.Watch, c.Quiet = ms(5), ms(40) }).subscribed()
	if err := b.app.Connect(Auto); err != nil {
		t.Fatal(err)
	}
	b.tunnel.Count(1000, 5000)
	time.Sleep(ms(150))
	if got := b.tunnel.Running(); got != "обход1" {
		t.Fatalf("traffic in both directions: running %q", got)
	}
	b.seen()
	b.tunnel.Kill("обход1")
	time.Sleep(ms(150))
	if got := b.tunnel.Running(); got != "обход1" {
		t.Fatalf("no traffic, nobody asked: running %q", got)
	}
	b.tunnel.Count(1400, 5000)
	time.Sleep(ms(150))
	if got := b.tunnel.Running(); got != "обход1" {
		t.Fatalf("one request without an answer: running %q", got)
	}
	b.tunnel.Count(1500, 5000)
	deadline := time.Now().Add(5 * time.Second)
	for b.tunnel.Running() != "nl" && time.Now().Before(deadline) {
		time.Sleep(ms(5))
	}
	if got := b.tunnel.Running(); got != "nl" {
		t.Fatalf("traffic went out and nothing came back: running %q", got)
	}
	for b.app.Snapshot().Connection.State != session.Connected && time.Now().Before(deadline) {
		time.Sleep(ms(5))
	}
	time.Sleep(ms(30))
	b.tunnel.Kill("nl")
	b.tunnel.Count(1800, 5000)
	time.Sleep(ms(30))
	b.tunnel.Count(1900, 5000)
	deadline = time.Now().Add(5 * time.Second)
	for b.tunnel.Running() == "nl" && time.Now().Before(deadline) {
		time.Sleep(ms(5))
	}
	if got := b.tunnel.Running(); got == "nl" {
		t.Fatal("the next server went silent right after the move: the pause of the first check held the second one back")
	}
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

func TestOldMeasurementIsRenewedBehindARunningTunnelAndTheReserveOrderFollowsIt(t *testing.T) {
	b := newBench(t, func(c *Config) { c.Bypass, c.Resample, c.Settle = true, time.Hour, ms(1) })
	b.tunnel.Answer(map[string]time.Duration{"nl": ms(40), "de": ms(80), "us": ms(120), "fl": ms(150), "pl": ms(160)})
	b.subscribed()
	if err := b.app.Connect(Auto); err != nil {
		t.Fatal(err)
	}
	time.Sleep(ms(60))
	if n := b.tunnel.Measured(); n != 1 {
		t.Fatalf("a measurement taken a moment ago was repeated: %d", n)
	}
	b.app.Disconnect()
	b.clock.Advance(2 * time.Hour)
	b.tunnel.Answer(map[string]time.Duration{"nl": ms(45), "de": ms(80), "us": ms(120), "fl": ms(150), "pl": ms(20)})
	b.seen()
	if err := b.app.Connect(Auto); err != nil {
		t.Fatal(err)
	}
	b.until("the new delay is saved and announced", func() bool { return find(b.app.Snapshot().Servers, "pl").DelayMs == 20 && b.heard(EvServers) })
	if got, w := b.tunnel.Running(), b.tunnel.Workers(); got != "nl" || len(w) != 2 || w[1] != 1 {
		t.Fatalf("running %q, workers %v", got, w)
	}
	same(t, b.seen(), []string{"connecting:nl", "connected:nl", EvServers})
	if pl := find(b.app.Snapshot().Servers, "pl"); pl.MeasuredAt != b.clock.Now().Unix() {
		t.Fatalf("server = %+v", pl)
	}
	b.tunnel.Kill("nl")
	if err := b.app.Check(); err != nil {
		t.Fatal(err)
	}
	if got := b.tunnel.Running(); got != "pl" {
		t.Fatalf("the reserve order did not follow the fresh measurement: running %q", got)
	}
	time.Sleep(ms(60))
	n := b.tunnel.Measured()
	b.app.Check()
	time.Sleep(ms(60))
	if b.tunnel.Measured() != n {
		t.Fatalf("measured again within the hour: %d, was %d", b.tunnel.Measured(), n)
	}
	b.clock.Advance(2 * time.Hour)
	b.app.Check()
	b.until("measured again after the hour", func() bool { return b.tunnel.Measured() == n+1 })
	if w := b.tunnel.Workers(); w[len(w)-1] != 1 {
		t.Fatalf("workers %v", w)
	}
}

func TestBackgroundMeasuringKeepsOutOfTheWay(t *testing.T) {
	for name, tune := range map[string]func(*Config){
		"the tunnel cannot be passed by": func(c *Config) { c.Resample, c.Settle = time.Hour, ms(1) },
		"switched off":                   func(c *Config) { c.Bypass, c.Resample, c.Settle = true, -1, ms(1) },
	} {
		b := newBench(t, tune).subscribed()
		if err := b.app.Connect(Auto); err != nil {
			t.Fatal(err)
		}
		b.app.Disconnect()
		b.clock.Advance(2 * time.Hour)
		if err := b.app.Connect(Auto); err != nil {
			t.Fatal(err)
		}
		time.Sleep(ms(60))
		if n := b.tunnel.Measured(); n != 1 {
			t.Fatalf("%s: measured %d times", name, n)
		}
	}
	b := newBench(t, func(c *Config) { c.Bypass, c.Resample, c.Settle = true, time.Hour, ms(1) }).subscribed()
	if err := b.app.Connect(Auto); err != nil {
		t.Fatal(err)
	}
	b.app.Disconnect()
	b.clock.Advance(2 * time.Hour)
	if err := b.app.Connect("nl"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(ms(60))
	if n := b.tunnel.Measured(); n != 1 {
		t.Fatalf("a server chosen by hand: measured %d times", n)
	}
	b.app.Disconnect()
	b.tunnel.Answer(map[string]time.Duration{"de": ms(80), "nl": ms(400), "us": ms(120), "обход1": ms(10)})
	release := b.tunnel.HoldMeasure()
	if err := b.app.Connect(Auto); err != nil {
		t.Fatal(err)
	}
	b.until("the background measuring starts", func() bool { return b.tunnel.Measured() == 2 })
	b.app.Disconnect()
	release()
	time.Sleep(ms(60))
	if nl := find(b.app.Snapshot().Servers, "nl"); nl.DelayMs != 40 {
		t.Fatalf("a measurement stopped by a disconnect was saved: %+v", nl)
	}
	b.clock.Advance(10 * time.Minute)
	release = b.tunnel.HoldMeasure()
	if err := b.app.Connect(Auto); err != nil {
		t.Fatal(err)
	}
	b.until("the background measuring starts again", func() bool { return b.tunnel.Measured() == 3 })
	done := make(chan error, 1)
	go func() { done <- b.app.Measure(context.Background()) }()
	b.until("the measuring asked by a person starts", func() bool { return b.tunnel.Measured() == 4 })
	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if nl, w := find(b.app.Snapshot().Servers, "nl"), b.tunnel.Workers(); nl.DelayMs != 400 || len(w) != 4 || w[1] != 1 || w[2] != 1 || w[3] == 1 {
		t.Fatalf("server %+v, workers %v", nl, w)
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

func TestBlockLeftByAnEarlierRunResumesTheConnection(t *testing.T) {
	var kept []bool
	held := func(c *Config) {
		c.Held = true
		c.Keep = func(on bool) { kept = append(kept, on) }
	}
	b := newBench(t, held)
	if err := b.app.Resume(); err != nil || len(b.tunnel.Starts()) != 0 || b.tunnel.Stops() != 1 {
		t.Fatalf("no subscription: err %v, starts %v, stops %d", err, b.tunnel.Starts(), b.tunnel.Stops())
	}
	b.subscribed()
	if err := b.app.Set(KeyKillSwitch, json.RawMessage(`true`)); err != nil {
		t.Fatal(err)
	}
	b.open(held)
	if err := b.app.Resume(); err != nil || b.tunnel.Running() != "обход1" || b.app.Snapshot().Connection.State != session.Connected {
		t.Fatalf("held without auto connect: err %v, running %q", err, b.tunnel.Running())
	}
	if err := b.app.Set(KeyKillSwitch, json.RawMessage(`false`)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kept, []bool{false, true, true, false}) {
		t.Fatalf("kept %v", kept)
	}
	if err := b.app.Disconnect(); err != nil {
		t.Fatal(err)
	}
	b.open(nil)
	if err := b.app.Resume(); err != nil || b.tunnel.Running() != "" {
		t.Fatalf("not held: err %v, running %q", err, b.tunnel.Running())
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
	if err := b.app.Set("theme", json.RawMessage(`["a.b","c.d"]`)); err != nil {
		t.Fatal(err)
	}
	same(t, b.seen(), []string{EvSettings, EvSettings})
	b.open(nil)
	if s := b.app.Snapshot().Settings; string(s["killSwitch"]) != "true" || string(s["theme"]) != `["a.b","c.d"]` {
		t.Fatalf("settings = %v", s)
	}
	if err := b.app.Set("theme", json.RawMessage(`null`)); err != nil {
		t.Fatal(err)
	}
	if s := b.app.Snapshot().Settings; len(s) != 1 {
		t.Fatalf("settings = %v", s)
	}
}

func TestChosenAppsAreCheckedAndKeptInOneForm(t *testing.T) {
	b := newBench(t, nil)
	many := make([]string, 513)
	for i := range many {
		many[i] = "org.example.app" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + string(rune('a'+i/26%26))
	}
	long, _ := json.Marshal(Apps{Mode: AppsExcept, List: many})
	for _, bad := range []string{
		`["org.example.one"]`,
		`"except"`,
		`{"mode":"some","list":["org.example.one"]}`,
		`{"mode":"only","list":[]}`,
		`{"mode":"only"}`,
		`{"mode":"except","list":["org.example.one"],"extra":1}`,
		`{"mode":"except","list":["org example"]}`,
		`{"mode":"except","list":[""]}`,
		`{"mode":"except","list":["org.example/../x"]}`,
		`{"mode":"except","list":["` + strings.Repeat("a", 256) + `"]}`,
		string(long),
	} {
		if err := b.app.Set(KeyApps, json.RawMessage(bad)); code(err) != CodeValue {
			t.Fatalf("%.60s: err = %v", bad, err)
		}
	}
	if b.app.Apps() != nil || len(b.app.Snapshot().Settings) != 0 {
		t.Fatalf("a refused value was kept: %v", b.app.Snapshot().Settings)
	}
	if err := b.app.Set(KeyApps, json.RawMessage(`{"list":["org.b","org.a","org.b","com.c_d.E2"],"mode":"only"}`)); err != nil {
		t.Fatal(err)
	}
	b.open(nil)
	if got := string(b.app.Snapshot().Settings[KeyApps]); got != `{"mode":"only","list":["com.c_d.E2","org.a","org.b"]}` {
		t.Fatalf("kept as %s", got)
	}
	if v := b.app.Apps(); v == nil || v.Mode != AppsOnly || len(v.List) != 3 {
		t.Fatalf("apps = %+v", v)
	}
	if err := b.app.Set(KeyApps, json.RawMessage(`{"mode":"all","list":["org.b","org.a"]}`)); err != nil {
		t.Fatal(err)
	}
	if got := string(b.app.Snapshot().Settings[KeyApps]); got != `{"mode":"all","list":["org.a","org.b"]}` || b.app.Apps() != nil {
		t.Fatalf("the list is kept for later and is not in force: %s, in force %+v", got, b.app.Apps())
	}
	for _, empty := range []string{`{"mode":"except","list":[]}`, `{"mode":"all","list":[]}`} {
		if err := b.app.Set(KeyApps, json.RawMessage(`{"mode":"except","list":["org.a"]}`)); err != nil {
			t.Fatal(err)
		}
		if err := b.app.Set(KeyApps, json.RawMessage(empty)); err != nil {
			t.Fatal(err)
		}
		if b.app.Apps() != nil || len(b.app.Snapshot().Settings) != 0 {
			t.Fatalf("%s is not the same as no setting: %v", empty, b.app.Snapshot().Settings)
		}
	}
}

func TestNewAppsChoiceIsAppliedToARunningTunnelAndOnlyToIt(t *testing.T) {
	var asked atomic.Int32
	var given atomic.Bool
	b := newBench(t, func(c *Config) {
		c.Retune = func() (bool, error) {
			asked.Add(1)
			return given.Load(), nil
		}
	}).subscribed()
	one := json.RawMessage(`{"mode":"except","list":["org.example.one"]}`)
	if err := b.app.Set(KeyApps, one); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if asked.Load() != 0 || len(b.tunnel.Starts()) != 0 {
		t.Fatalf("a choice made while disconnected touched the tunnel: asked %d, starts %v", asked.Load(), b.tunnel.Starts())
	}
	if err := b.app.Connect("nl"); err != nil {
		t.Fatal(err)
	}
	if err := b.app.Set(KeyApps, json.RawMessage(`{"mode":"except","list":["org.example.two"]}`)); err != nil {
		t.Fatal(err)
	}
	b.until("the platform is asked for a new tunnel", func() bool { return asked.Load() == 1 })
	time.Sleep(30 * time.Millisecond)
	if got := b.tunnel.Starts(); !reflect.DeepEqual(got, []string{"nl"}) {
		t.Fatalf("no new tunnel came, yet the engine was restarted: %v", got)
	}
	given.Store(true)
	if err := b.app.Set(KeyApps, one); err != nil {
		t.Fatal(err)
	}
	b.until("the engine is started again on the new tunnel", func() bool {
		return asked.Load() == 2 && reflect.DeepEqual(b.tunnel.Starts(), []string{"nl", "nl"}) && b.app.Snapshot().Connection.State == session.Connected
	})
	for _, same := range []string{`"dark"`, `{"list":["org.example.one"],"mode":"except"}`} {
		key := "theme"
		if strings.HasPrefix(same, "{") {
			key = KeyApps
		}
		if err := b.app.Set(key, json.RawMessage(same)); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(30 * time.Millisecond)
	if asked.Load() != 2 {
		t.Fatal("another setting, or the same choice again, asked for a new tunnel")
	}
	if err := b.app.Set(KeyApps, json.RawMessage(`{"mode":"all","list":["org.example.one"]}`)); err != nil {
		t.Fatal(err)
	}
	b.until("every app is back in the tunnel by a new one", func() bool {
		return asked.Load() == 3 && reflect.DeepEqual(b.tunnel.Starts(), []string{"nl", "nl", "nl"}) && b.app.Snapshot().Connection.State == session.Connected
	})
	if err := b.app.Set(KeyApps, json.RawMessage(`{"mode":"all","list":["org.example.two"]}`)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if asked.Load() != 3 {
		t.Fatal("a list that is not in force asked for a new tunnel")
	}
}

func TestCloseWaitsForWhatRunsInTheBackground(t *testing.T) {
	entered, leave := make(chan struct{}), make(chan struct{})
	b := newBench(t, func(c *Config) {
		c.Retune = func() (bool, error) {
			close(entered)
			<-leave
			return false, nil
		}
	}).subscribed()
	if err := b.app.Connect("nl"); err != nil {
		t.Fatal(err)
	}
	if err := b.app.Set(KeyApps, json.RawMessage(`{"mode":"except","list":["org.a"]}`)); err != nil {
		t.Fatal(err)
	}
	<-entered
	closed := make(chan struct{})
	go func() {
		b.app.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("closed while a background task was still running")
	case <-time.After(ms(80)):
	}
	close(leave)
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("did not close after the background task ended")
	}
	b.app.Check()
	time.Sleep(ms(30))
}

func TestUnsubscribe(t *testing.T) {
	b := newBench(t, nil).subscribed()
	if err := b.app.Connect("de"); err != nil {
		t.Fatal(err)
	}
	b.seen()
	if err := b.app.Unsubscribe(""); err != nil {
		t.Fatal(err)
	}
	same(t, b.seen(), []string{"idle", "idle", EvSubscription, EvServers, EvList})
	snap := b.app.Snapshot()
	if snap.Subscription != nil || len(snap.Servers) != 0 || len(snap.List) != 0 || snap.Connection.Choice != Auto || b.tunnel.Running() != "" {
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
	if tr := b.app.Traffic(); tr != (engine.Traffic{Up: 10, Down: 20}) {
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

func TestReportKeepsAShortJournalOfChangesAndNothingSecret(t *testing.T) {
	b := newBench(t, nil).subscribed()
	if err := b.app.Connect(Auto); err != nil {
		t.Fatal(err)
	}
	b.tunnel.Answer(map[string]time.Duration{"de": ms(80), "nl": ms(40), "us": ms(120), "обход1": ms(15)})
	if err := b.app.Check(); err != nil {
		t.Fatal(err)
	}
	r := b.app.Report()
	var got []string
	for _, e := range r.Journal {
		got = append(got, string(e.State)+":"+e.Server)
	}
	same(t, got, []string{"measuring:", "connecting:обход1", "connected:обход1"})
	if r.Device == "" || r.OS == "" || r.StartedAt == 0 || r.Journal[0].At == 0 {
		t.Fatalf("report = %+v", r)
	}
	for i := 0; i < journalSize; i++ {
		if err := b.app.Disconnect(); err != nil {
			t.Fatal(err)
		}
		if err := b.app.Connect(Auto); err != nil {
			t.Fatal(err)
		}
	}
	r = b.app.Report()
	if last := r.Journal[len(r.Journal)-1]; len(r.Journal) != journalSize || last.State != session.Connected {
		t.Fatalf("%d entries, the last is %+v", len(r.Journal), last)
	}
	text, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{apptest.Address, "outbounds", "vnext"} {
		if strings.Contains(string(text), secret) {
			t.Fatalf("the report holds %q", secret)
		}
	}
}

func notices(lines ...string) []byte {
	var out []json.RawMessage
	for _, line := range lines {
		name, _ := json.Marshal(line)
		out = append(out, json.RawMessage(`{"remarks":`+string(name)+`,"outbounds":[{"tag":"proxy","protocol":"vless","settings":{"vnext":[{"address":"0.0.0.0","port":1,"users":[{"id":"00000000-0000-0000-0000-000000000000","encryption":"none"}]}]},"streamSettings":{"network":"tcp"}},{"tag":"direct","protocol":"freedom"}]}`))
	}
	body, _ := json.Marshal(out)
	return body
}

func (b *bench) answer(etag, usage string, body []byte, flags ...string) {
	b.panel.Set(func(was *[]byte, h http.Header) {
		*was = body
		h.Set("Etag", etag)
		h.Set("Subscription-Userinfo", usage)
		h.Del("X-Hwid-Max-Devices-Reached")
		for _, f := range flags {
			h.Set(f, "true")
		}
	})
}

func TestASubscriptionWithoutServersSaysWhyAndIsNotConnected(t *testing.T) {
	b := newBench(t, nil)
	full, _ := apptest.Fixture(t)
	lines := []string{"⏰ Подписка закончилась", "VPN выключен до продления"}
	b.answer(`W/"ended"`, "upload=0; download=0; total=5368709120; expire=1794825204", notices(lines...))
	if err := b.app.Subscribe(context.Background(), apptest.Address); err != nil {
		t.Fatal(err)
	}
	snap := b.app.Snapshot()
	if s := snap.Subscription; s.Stop != StopEnded || !reflect.DeepEqual(s.Notes, lines) || len(snap.Servers) != 0 {
		t.Fatalf("stop %q, notes %q, servers %d", s.Stop, s.Notes, len(snap.Servers))
	}
	if err := b.app.Connect(Auto); code(err) != "sub_ended" {
		t.Fatalf("connect with nothing to connect to: %v", err)
	}
	if err := b.app.Measure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := b.tunnel.Starts(); len(got) != 0 {
		t.Fatalf("something was started: %v", got)
	}
	for _, c := range []struct {
		etag, usage string
		flags       []string
		want        Stop
	}{
		{`W/"spent"`, "upload=1; download=10; total=11; expire=1798761600", nil, StopSpent},
		{`W/"devices"`, "upload=1; download=10; total=0; expire=1798761600", []string{"X-Hwid-Max-Devices-Reached"}, StopDevices},
		{`W/"other"`, "upload=1; download=10; total=0; expire=0", nil, StopOther},
	} {
		b.answer(c.etag, c.usage, notices("x"), c.flags...)
		if _, err := b.app.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
		if got := b.app.Snapshot().Subscription.Stop; got != c.want {
			t.Fatalf("%s: stop %q, want %q", c.etag, got, c.want)
		}
	}
	b.answer(`W/"paid"`, "upload=1; download=10; total=0; expire=1798761600", full)
	if err := b.app.Connect(Auto); err != nil || b.tunnel.Running() == "" {
		t.Fatalf("connect after a payment made elsewhere asks the panel first: err %v, running %q", err, b.tunnel.Running())
	}
	if s := b.app.Snapshot().Subscription; s.Stop != "" || len(s.Notes) != 0 {
		t.Fatalf("after the payment: stop %q, notes %q", s.Stop, s.Notes)
	}
}

func TestARunningConnectionEndsWhenTheSubscriptionGivesNoServers(t *testing.T) {
	b := newBench(t, renewing).subscribed()
	if err := b.app.Connect(Auto); err != nil {
		t.Fatal(err)
	}
	b.seen()
	b.answer(`W/"ended"`, "upload=0; download=0; total=0; expire=1794825204", notices("⏰ Подписка закончилась"))
	if changed, err := b.app.Refresh(context.Background()); err != nil || !changed {
		t.Fatalf("changed %v, err %v", changed, err)
	}
	b.until("the tunnel is taken down", func() bool {
		return b.app.Snapshot().Connection.State == session.Idle && b.tunnel.Running() == ""
	})
	if got := b.seen(); len(got) < 3 || got[0] != EvSubscription || got[1] != EvServers || got[len(got)-1] != string(session.Idle) {
		t.Fatalf("events %v", got)
	}
	if s := b.app.Snapshot().Subscription; s.Stop != StopEnded {
		t.Fatalf("stop %q", s.Stop)
	}
}

func TestAnAddressThatAnswersWithLinksIsAskedAgainForJSON(t *testing.T) {
	b := newBench(t, nil)
	b.panel.Links()
	if err := b.app.Subscribe(context.Background(), apptest.Address+"/"); err != nil {
		t.Fatal(err)
	}
	if len(b.app.Snapshot().Servers) == 0 {
		t.Fatal("no servers")
	}
	b.panel.Set(func(_ *[]byte, h http.Header) { h.Set("Etag", `W/"next"`) })
	if _, err := b.app.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := strings.TrimPrefix(apptest.Address, "https://sub.example.test")
	same(t, b.panel.Paths(), []string{path + "/", path + "/json", path + "/json"})
	b2 := newBench(t, nil)
	b2.panel.Links()
	b2.panel.Set(func(body *[]byte, _ http.Header) { *body = []byte("<html>") })
	if err := b2.app.Subscribe(context.Background(), apptest.Address); code(err) != CodeFormat {
		t.Fatalf("neither form is a list: %v", err)
	}
}

const second = "https://sub.example.test/sub/SECOND0123456789"

func (b *bench) only(names ...string) {
	b.panel.Set(func(body *[]byte, _ http.Header) {
		var all []map[string]any
		json.Unmarshal(*body, &all)
		kept := []map[string]any{}
		for _, s := range all {
			for _, n := range names {
				if strings.Contains(s["remarks"].(string), "("+n+")") {
					kept = append(kept, s)
				}
			}
		}
		*body, _ = json.Marshal(kept)
	})
}

func TestASecondSubscriptionKeepsTheFirstWithItsChoice(t *testing.T) {
	b := newBench(t, nil).subscribed()
	if err := b.app.Connect("de"); err != nil {
		t.Fatal(err)
	}
	full, _ := apptest.Fixture(t)
	b.only("NL", "US")
	if err := b.app.Subscribe(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	snap := b.app.Snapshot()
	if len(snap.List) != 2 || !snap.List[0].Active || snap.List[0].Servers != 2 || snap.List[1].Active || snap.List[1].Servers != 19 || len(snap.Servers) != 2 || snap.Connection.Choice != Auto {
		t.Fatalf("after the second: %+v", snap)
	}
	if !waitFor(func() bool { r := b.tunnel.Running(); return r == "nl" || r == "us" }) {
		t.Fatalf("the tunnel stayed on %q after the second subscription took over", b.tunnel.Running())
	}
	if err := b.app.Connect("us"); err != nil {
		t.Fatal(err)
	}
	b.panel.Set(func(body *[]byte, _ http.Header) { *body = full })
	b.seen()
	if err := b.app.Activate(snap.List[1].ID); err != nil {
		t.Fatal(err)
	}
	seen := b.seen()
	if !slices.Contains(seen, EvList) || !slices.Contains(seen, EvServers) {
		t.Fatalf("events %v", seen)
	}
	back := b.app.Snapshot()
	if len(back.Servers) != 19 || back.Connection.Choice != "de" || !back.List[0].Active || back.List[0].ID != snap.List[1].ID || back.List[1].ID != snap.List[0].ID {
		t.Fatalf("after switching back: choice %q list %+v", back.Connection.Choice, back.List)
	}
	if !waitFor(func() bool { return b.tunnel.Running() == "de" }) {
		t.Fatalf("running %q, want the server chosen in the first subscription", b.tunnel.Running())
	}
	if err := b.app.Activate(back.List[0].ID); err != nil {
		t.Fatal("activating the active subscription must be a no-op")
	}
	if err := b.app.Activate("000000000000"); code(err) != CodeUnknown {
		t.Fatalf("unknown id: %v", err)
	}
	b.open(nil)
	if again := b.app.Snapshot(); len(again.List) != 2 || again.List[0].ID != back.List[0].ID || again.Connection.Choice != "de" {
		t.Fatalf("after restart: %+v", again.List)
	}
	if err := b.app.Subscribe(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if again := b.app.Snapshot(); len(again.List) != 2 || again.List[0].ID != snap.List[0].ID || again.Connection.Choice != "us" {
		t.Fatalf("subscribing again to a known address must bring it back with its choice, not add a copy: %+v choice %q", again.List, again.Connection.Choice)
	}
}

func TestUnsubscribeOneOfSeveral(t *testing.T) {
	b := newBench(t, nil).subscribed()
	b.only("NL")
	if err := b.app.Subscribe(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	snap := b.app.Snapshot()
	b.seen()
	if err := b.app.Unsubscribe(snap.List[1].ID); err != nil {
		t.Fatal(err)
	}
	same(t, b.seen(), []string{EvList})
	if now := b.app.Snapshot(); len(now.List) != 1 || now.List[0].ID != snap.List[0].ID || len(now.Servers) != 1 {
		t.Fatalf("after removing the parked one: %+v", now)
	}
	if err := b.app.Unsubscribe(snap.List[1].ID); code(err) != CodeUnknown {
		t.Fatalf("removing it twice: %v", err)
	}
	if err := b.app.Subscribe(context.Background(), apptest.Address); err != nil {
		t.Fatal(err)
	}
	if err := b.app.Connect("nl"); err != nil {
		t.Fatal(err)
	}
	if err := b.app.Unsubscribe(""); err != nil {
		t.Fatal(err)
	}
	now := b.app.Snapshot()
	if b.tunnel.Running() != "" || now.Connection.State != session.Idle || len(now.List) != 1 || !now.List[0].Active || len(now.Servers) != 1 || now.Subscription == nil {
		t.Fatalf("removing the active one must turn the VPN off and bring the next one: running %q %+v", b.tunnel.Running(), now)
	}
}

func TestSwitchingToASubscriptionWithoutServersTurnsTheVPNOff(t *testing.T) {
	b := newBench(t, nil).subscribed()
	b.panel.Set(func(body *[]byte, _ http.Header) { *body = notices("⏰ Подписка закончилась") })
	if err := b.app.Subscribe(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	snap := b.app.Snapshot()
	if snap.List[0].Stop == "" || snap.List[0].Servers != 0 {
		t.Fatalf("list = %+v", snap.List)
	}
	if err := b.app.Activate(snap.List[1].ID); err != nil {
		t.Fatal(err)
	}
	if err := b.app.Connect("de"); err != nil {
		t.Fatal(err)
	}
	if err := b.app.Activate(snap.List[0].ID); err != nil {
		t.Fatal(err)
	}
	if !waitFor(func() bool { return b.tunnel.Running() == "" && b.app.Snapshot().Connection.State == session.Idle }) {
		t.Fatalf("the VPN stayed on %q after switching to a subscription without servers", b.tunnel.Running())
	}
}

func TestNoMoreThanTwentySubscriptionsAreKept(t *testing.T) {
	b := newBench(t, nil)
	for i := range maxSubscriptions + 3 {
		if err := b.app.Subscribe(context.Background(), fmt.Sprintf("%s/%d", second, i)); err != nil {
			t.Fatal(err)
		}
	}
	snap := b.app.Snapshot()
	if len(snap.List) != maxSubscriptions || !snap.List[0].Active {
		t.Fatalf("kept %d", len(snap.List))
	}
}

func waitFor(ok func() bool) bool {
	for range 200 {
		if ok() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return ok()
}
