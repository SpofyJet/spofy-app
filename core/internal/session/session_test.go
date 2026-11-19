package session

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/engine"
	"github.com/SpofyJet/spofy-app/core/internal/supervisor"
)

type fake struct {
	mu       sync.Mutex
	delays   map[string]time.Duration
	dead     map[string]bool
	broken   map[string]bool
	flaky    int
	gate     chan struct{}
	running  string
	log      []string
	measured int
	probes   int
	limits   []time.Duration
}

func (f *fake) Start(cfg []byte) (time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = append(f.log, "start:"+string(cfg))
	if f.broken[string(cfg)] {
		f.running = ""
		return 0, &engine.Error{Code: engine.CodeConfig, Err: errors.New("bad config")}
	}
	f.running = string(cfg)
	return time.Millisecond, nil
}

func (f *fake) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = append(f.log, "stop")
	f.running = ""
	return nil
}

func (f *fake) Probe(ctx context.Context, _, _ string, limit time.Duration) (time.Duration, error) {
	f.mu.Lock()
	f.probes++
	f.limits = append(f.limits, limit)
	gate, running := f.gate, f.running
	fail := f.dead[running]
	if f.flaky > 0 {
		f.flaky--
		fail = true
	}
	delay := f.delays[running]
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return 0, &engine.Error{Code: supervisor.CodeCanceled, Err: ctx.Err()}
		}
	}
	if running == "" {
		return 0, &engine.Error{Code: supervisor.CodeUnavailable, Err: errors.New("engine is not running")}
	}
	if fail {
		return 0, &engine.Error{Code: engine.CodeProbe, Err: errors.New("no answer")}
	}
	return delay, nil
}

func (f *fake) Measure(ctx context.Context, targets []supervisor.Target, _ supervisor.MeasureOptions, each func(int, supervisor.Measurement)) {
	f.mu.Lock()
	f.measured++
	f.mu.Unlock()
	for i, t := range targets {
		f.mu.Lock()
		delay, ok := f.delays[string(t.Config)]
		f.mu.Unlock()
		if !ok {
			each(i, supervisor.Measurement{Code: engine.CodeProbe, Error: "no answer"})
			continue
		}
		each(i, supervisor.Measurement{Delay: delay})
	}
}

func (f *fake) set(change func()) {
	f.mu.Lock()
	change()
	f.mu.Unlock()
}

func (f *fake) starts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, l := range f.log {
		if l != "stop" {
			out = append(out, l[len("start:"):])
		}
	}
	return out
}

func (f *fake) now() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running
}

func (f *fake) probed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.probes
}

type trace struct {
	mu   sync.Mutex
	list []Event
}

func (t *trace) on(e Event) {
	t.mu.Lock()
	t.list = append(t.list, e)
	t.mu.Unlock()
}

func (t *trace) states() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []string
	for _, e := range t.list {
		s := string(e.State)
		if e.Server != "" {
			s += ":" + e.Server
		}
		out = append(out, s)
	}
	return out
}

func known(list []Candidate, delays map[string]time.Duration) []Candidate {
	for i := range list {
		list[i].Known = delays[list[i].ID]
	}
	return list
}

func (f *fake) stops() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, l := range f.log {
		if l == "stop" {
			n++
		}
	}
	return n
}

func cands(ids ...string) []Candidate {
	var out []Candidate
	for _, id := range ids {
		out = append(out, Candidate{ID: id, Run: []byte(id), Probe: []byte(id), Tag: "proxy"})
	}
	return out
}

func setup(f *fake, o Options) (*Session, *trace) {
	tr := &trace{}
	o.Tunnel, o.OnEvent = f, tr.on
	if o.Retries == nil {
		o.Retries = []time.Duration{0}
	}
	return New(o), tr
}

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func speeds(de, nl, us int) map[string]time.Duration {
	return map[string]time.Duration{"de": ms(de), "nl": ms(nl), "us": ms(us)}
}

func down(ids ...string) map[string]bool {
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func codeOf(err error) engine.Code {
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

func TestOrder(t *testing.T) {
	r := []Result{{Delay: ms(90), OK: true}, {Delay: ms(40), OK: true}, {Delay: ms(10)}, {Tier: 1, Delay: ms(5), OK: true}, {Delay: ms(45), OK: true}}
	if got := Order(r, -1); !reflect.DeepEqual(got, []int{1, 4, 0, 3}) {
		t.Fatalf("order = %v", got)
	}
	if got := Order(r, 4); !reflect.DeepEqual(got, []int{4, 1, 0, 3}) {
		t.Fatalf("close second must stay first: %v", got)
	}
	if got := Order(r, 0); !reflect.DeepEqual(got, []int{1, 4, 0, 3}) {
		t.Fatalf("much slower server must not stick: %v", got)
	}
	for _, keep := range []int{2, 3, 9} {
		if got := Order(r, keep); !reflect.DeepEqual(got, []int{1, 4, 0, 3}) {
			t.Fatalf("keep %d: %v", keep, got)
		}
	}
	if got := Order([]Result{{}, {}}, 0); len(got) != 0 {
		t.Fatalf("nothing answered: %v", got)
	}
}

func TestAutoPicksFastestAnswering(t *testing.T) {
	f := &fake{delays: speeds(80, 40, 120)}
	s, tr := setup(f, Options{})
	if err := s.Connect(context.Background(), cands("de", "nl", "us", "by"), ""); err != nil {
		t.Fatal(err)
	}
	same(t, tr.states(), []string{"measuring", "connecting:nl", "connected:nl"})
	if s.State() != Connected || f.now() != "nl" {
		t.Fatalf("state %s, running %q", s.State(), f.now())
	}
}

func TestAutoSkipsServersThatFailInTheTunnel(t *testing.T) {
	f := &fake{delays: speeds(80, 40, 20), dead: down("nl"), broken: down("us")}
	s, tr := setup(f, Options{})
	if err := s.Connect(context.Background(), cands("de", "nl", "us"), ""); err != nil {
		t.Fatal(err)
	}
	same(t, f.starts(), []string{"us", "nl", "de"})
	same(t, tr.states(), []string{"measuring", "connecting:us", "connecting:nl", "connecting:de", "connected:de"})
}

func TestAutoFailsWhenNothingAnswers(t *testing.T) {
	f := &fake{}
	s, tr := setup(f, Options{})
	if err := s.Connect(context.Background(), cands("de", "nl"), ""); codeOf(err) != CodeNoServer {
		t.Fatalf("err = %v", err)
	}
	same(t, tr.states(), []string{"measuring", "failed"})
	if len(f.starts()) != 0 {
		t.Fatalf("tunnel was started: %v", f.starts())
	}
	f.set(func() { f.delays, f.dead = speeds(10, 20, 30), down("de", "nl") })
	if err := s.Connect(context.Background(), cands("de", "nl"), ""); codeOf(err) != CodeNoServer {
		t.Fatalf("err = %v", err)
	}
	if f.now() != "" || s.State() != Failed {
		t.Fatalf("running %q, state %s", f.now(), s.State())
	}
	if err := s.Check(context.Background()); err != nil || len(f.starts()) != 2 {
		t.Fatalf("check after a failed connect: err %v, starts %v", err, f.starts())
	}
}

func TestStrictKeepsTheBlockWhenConnectFails(t *testing.T) {
	f := &fake{delays: speeds(10, 20, 30), dead: down("de", "nl")}
	s, _ := setup(f, Options{Strict: true})
	if err := s.Connect(context.Background(), cands("de", "nl"), ""); codeOf(err) != CodeNoServer {
		t.Fatalf("err = %v", err)
	}
	if f.now() == "" || s.State() != Stalled {
		t.Fatalf("running %q, state %s", f.now(), s.State())
	}
	f.set(func() { f.dead = down("nl") })
	if err := s.Check(context.Background()); err != nil || s.State() != Connected || f.now() != "de" {
		t.Fatalf("err %v, state %s, running %q", err, s.State(), f.now())
	}
}

func TestReserveTierOnlyWhenMainTierIsDown(t *testing.T) {
	c := cands("de", "nl", "bypass")
	c[2].Tier = 1
	f := &fake{delays: map[string]time.Duration{"de": ms(80), "nl": ms(90), "bypass": ms(5)}}
	s, _ := setup(f, Options{})
	if err := s.Connect(context.Background(), c, ""); err != nil || f.now() != "de" {
		t.Fatalf("err %v, running %q", err, f.now())
	}
	f.set(func() { f.dead = down("de", "nl") })
	if err := s.Check(context.Background()); err != nil || f.now() != "bypass" {
		t.Fatalf("err %v, running %q", err, f.now())
	}
}

func TestPinnedServer(t *testing.T) {
	f := &fake{delays: speeds(80, 40, 120)}
	s, tr := setup(f, Options{})
	if err := s.Connect(context.Background(), cands("de", "nl"), "de"); err != nil {
		t.Fatal(err)
	}
	same(t, tr.states(), []string{"connecting:de", "connected:de"})
	if f.measured != 0 {
		t.Fatal("a pinned server must not trigger measuring")
	}
	if err := s.Connect(context.Background(), cands("de", "nl"), "xx"); codeOf(err) != CodeNotFound {
		t.Fatalf("err = %v", err)
	}
	if f.now() != "de" || s.State() != Connected {
		t.Fatalf("unknown server broke the connection: running %q, state %s", f.now(), s.State())
	}
	f.set(func() { f.dead = down("nl") })
	if err := s.Connect(context.Background(), cands("de", "nl"), "nl"); codeOf(err) != CodeUnreachable {
		t.Fatalf("err = %v", err)
	}
	if f.now() != "nl" || s.State() != Stalled {
		t.Fatalf("a failed switch dropped the tunnel: running %q, state %s", f.now(), s.State())
	}
	f.set(func() { f.dead = nil })
	if err := s.Check(context.Background()); err != nil || s.State() != Connected || f.now() != "nl" {
		t.Fatalf("err %v, state %s, running %q", err, s.State(), f.now())
	}
}

func TestFirstConnectionToSilentServerLeavesNothingRunning(t *testing.T) {
	f := &fake{delays: speeds(80, 40, 120), dead: down("nl")}
	s, tr := setup(f, Options{})
	if err := s.Connect(context.Background(), cands("de", "nl"), "nl"); codeOf(err) != CodeUnreachable {
		t.Fatalf("err = %v", err)
	}
	same(t, tr.states(), []string{"connecting:nl", "failed"})
	if f.now() != "" || s.State() != Failed {
		t.Fatalf("running %q, state %s", f.now(), s.State())
	}
}

func TestCheckSwitchesToNextServer(t *testing.T) {
	f := &fake{delays: speeds(80, 40, 120)}
	s, tr := setup(f, Options{})
	if err := s.Connect(context.Background(), cands("de", "nl", "us"), ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	same(t, f.starts(), []string{"nl"})
	f.set(func() { f.dead = down("nl", "de") })
	if err := s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	same(t, f.starts(), []string{"nl", "de", "us"})
	same(t, tr.states()[3:], []string{"connected:nl", "switching:de", "switching:us", "connected:us"})
}

func TestTunnelStaysWhenEveryServerIsLost(t *testing.T) {
	f := &fake{delays: speeds(80, 40, 120)}
	s, tr := setup(f, Options{})
	if err := s.Connect(context.Background(), cands("de", "nl", "us"), ""); err != nil {
		t.Fatal(err)
	}
	f.set(func() { f.dead = down("nl", "de", "us") })
	if err := s.Check(context.Background()); codeOf(err) != CodeNoServer {
		t.Fatalf("err = %v", err)
	}
	if f.now() != "nl" || s.State() != Stalled {
		t.Fatalf("running %q, state %s", f.now(), s.State())
	}
	same(t, tr.states()[3:], []string{"switching:de", "switching:us", "switching:nl", "stalled"})
	f.set(func() { f.dead = down("nl", "us") })
	if err := s.Check(context.Background()); err != nil || s.State() != Connected || f.now() != "de" {
		t.Fatalf("err %v, state %s, running %q", err, s.State(), f.now())
	}
}

func TestCheckRetriesBeforeSwitching(t *testing.T) {
	f := &fake{delays: speeds(80, 40, 120)}
	s, _ := setup(f, Options{Retries: []time.Duration{0, ms(1), ms(1)}})
	if err := s.Connect(context.Background(), cands("de", "nl"), ""); err != nil {
		t.Fatal(err)
	}
	f.set(func() { f.flaky = 2 })
	if err := s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	same(t, f.starts(), []string{"nl"})
}

func TestOneAttemptPerServerWhenThereAreOthers(t *testing.T) {
	f := &fake{delays: speeds(80, 40, 120), dead: down("nl")}
	s, _ := setup(f, Options{Retries: []time.Duration{0, ms(1), ms(1)}})
	if err := s.Connect(context.Background(), cands("de", "nl", "us"), ""); err != nil || f.now() != "de" {
		t.Fatalf("err %v, running %q", err, f.now())
	}
	if f.probed() != 2 {
		t.Fatalf("probes = %d, want one per server", f.probed())
	}
	if err := s.Connect(context.Background(), cands("de", "nl", "us"), "nl"); codeOf(err) != CodeUnreachable {
		t.Fatalf("err = %v", err)
	}
	if f.probed() != 5 {
		t.Fatalf("probes = %d, want every retry for the only server", f.probed())
	}
}

func TestCheckDoesNotSwitchWhenOffline(t *testing.T) {
	online := false
	f := &fake{delays: speeds(80, 40, 120)}
	s, _ := setup(f, Options{Online: func() bool { return online }})
	if err := s.Connect(context.Background(), cands("de", "nl"), ""); err != nil {
		t.Fatal(err)
	}
	f.set(func() { f.dead = down("nl") })
	if err := s.Check(context.Background()); err == nil || s.State() != Offline {
		t.Fatalf("err %v, state %s", err, s.State())
	}
	same(t, f.starts(), []string{"nl"})
	f.set(func() { f.dead = nil })
	online = true
	if err := s.Check(context.Background()); err != nil || s.State() != Connected || f.now() != "nl" {
		t.Fatalf("err %v, state %s, running %q", err, s.State(), f.now())
	}
}

func TestPinnedServerStallsInsteadOfSwitching(t *testing.T) {
	f := &fake{delays: speeds(80, 40, 120)}
	s, _ := setup(f, Options{})
	if err := s.Connect(context.Background(), cands("de", "nl"), "de"); err != nil {
		t.Fatal(err)
	}
	f.set(func() { f.dead = down("de") })
	if err := s.Check(context.Background()); codeOf(err) != CodeUnreachable || s.State() != Stalled || f.now() != "de" {
		t.Fatalf("err %v, state %s, running %q", err, s.State(), f.now())
	}
	f.set(func() { f.dead = nil })
	if err := s.Check(context.Background()); err != nil || s.State() != Connected {
		t.Fatalf("err %v, state %s", err, s.State())
	}
	same(t, f.starts(), []string{"de"})
}

func TestStoppedEngineIsStartedOnTheSameServer(t *testing.T) {
	for _, pinned := range []string{"", "nl"} {
		f := &fake{delays: speeds(80, 40, 120)}
		s, _ := setup(f, Options{})
		if err := s.Connect(context.Background(), cands("de", "nl", "us"), pinned); err != nil {
			t.Fatal(err)
		}
		f.set(func() { f.running = "" })
		if err := s.Check(context.Background()); err != nil || s.State() != Connected {
			t.Fatalf("pinned %q: err %v, state %s", pinned, err, s.State())
		}
		same(t, f.starts(), []string{"nl", "nl"})
	}
}

func TestReconnectToBrokenListKeepsTheBlock(t *testing.T) {
	f := &fake{delays: speeds(80, 40, 10), broken: down("xx")}
	s, _ := setup(f, Options{Strict: true})
	if err := s.Connect(context.Background(), cands("de", "nl", "us"), ""); err != nil || f.now() != "us" {
		t.Fatalf("err %v, running %q", err, f.now())
	}
	if err := s.Connect(context.Background(), cands("xx"), "xx"); codeOf(err) != CodeUnreachable || s.State() != Stalled {
		t.Fatalf("err %v, state %s", err, s.State())
	}
	if err := s.Check(context.Background()); codeOf(err) != CodeUnreachable || s.State() != Stalled {
		t.Fatalf("check: err %v, state %s", err, s.State())
	}
	f.set(func() { f.broken = nil })
	if err := s.Check(context.Background()); err != nil || s.State() != Connected || f.now() != "xx" {
		t.Fatalf("err %v, state %s, running %q", err, s.State(), f.now())
	}
}

func TestDisconnectInterruptsConnect(t *testing.T) {
	f := &fake{delays: speeds(80, 40, 120), gate: make(chan struct{})}
	s, _ := setup(f, Options{})
	done := make(chan error, 1)
	go func() { done <- s.Connect(context.Background(), cands("de", "nl"), "") }()
	for len(f.starts()) == 0 {
		time.Sleep(time.Millisecond)
	}
	began := time.Now()
	if err := s.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(began); took > time.Second {
		t.Fatalf("disconnect waited %v", took)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("connect err = %v", err)
	}
	if f.now() != "" || s.State() != Idle {
		t.Fatalf("running %q, state %s", f.now(), s.State())
	}
	same(t, f.starts(), []string{"nl"})
	if err := s.Check(context.Background()); err != nil || f.probed() != 1 {
		t.Fatalf("check after disconnect: err %v, probes %d", err, f.probed())
	}
}

func TestCheckDoesNotPreemptConnect(t *testing.T) {
	gate := make(chan struct{})
	f := &fake{delays: speeds(80, 40, 120), gate: gate}
	s, _ := setup(f, Options{})
	done := make(chan error, 1)
	go func() { done <- s.Connect(context.Background(), cands("de"), "") }()
	for len(f.starts()) == 0 {
		time.Sleep(time.Millisecond)
	}
	if err := s.Check(context.Background()); err != nil {
		t.Fatalf("check during connect: %v", err)
	}
	close(gate)
	if err := <-done; err != nil || s.State() != Connected {
		t.Fatalf("connect err %v, state %s", err, s.State())
	}
}

func TestCheckYieldsToWaitingConnect(t *testing.T) {
	f := &fake{delays: speeds(80, 40, 120)}
	s, _ := setup(f, Options{})
	if err := s.Connect(context.Background(), cands("de"), ""); err != nil {
		t.Fatal(err)
	}
	before := f.probed()
	epoch := s.preempt()
	if err := s.Check(context.Background()); err != nil || f.probed() != before {
		t.Fatalf("check ran ahead of a waiting connect: err %v, probes %d", err, f.probed()-before)
	}
	if epoch.Err() != nil {
		t.Fatal("check canceled the waiting connect")
	}
	s.acquire()
	s.op.Unlock()
}

func TestSessionOutlivesTheConnectContext(t *testing.T) {
	f := &fake{delays: speeds(80, 40, 120)}
	s, _ := setup(f, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Connect(ctx, cands("de", "nl"), ""); err != nil {
		t.Fatal(err)
	}
	cancel()
	f.set(func() { f.dead = down("nl") })
	if err := s.Check(context.Background()); err != nil || f.now() != "de" {
		t.Fatalf("err %v, running %q", err, f.now())
	}
}

func TestCheckWaitsNoLongerThanTheServerNeeds(t *testing.T) {
	for _, c := range []struct{ delay, want time.Duration }{{ms(40), 2 * time.Second}, {ms(700), 3500 * time.Millisecond}, {2 * time.Second, 4 * time.Second}} {
		f := &fake{delays: map[string]time.Duration{"de": c.delay}}
		s, _ := setup(f, Options{})
		if err := s.Connect(context.Background(), cands("de"), "de"); err != nil {
			t.Fatal(err)
		}
		if err := s.Check(context.Background()); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(f.limits, []time.Duration{4 * time.Second, c.want}) {
			t.Fatalf("delay %v: probe limits = %v", c.delay, f.limits)
		}
	}
}

func TestRememberedOrderConnectsWithoutMeasuring(t *testing.T) {
	f := &fake{delays: speeds(80, 40, 120)}
	s, tr := setup(f, Options{})
	list := known(cands("de", "nl", "us"), map[string]time.Duration{"de": ms(80), "nl": ms(40)})
	if err := s.Connect(context.Background(), list, ""); err != nil {
		t.Fatal(err)
	}
	same(t, tr.states(), []string{"connecting:nl", "connected:nl"})
	if f.measured != 0 || !reflect.DeepEqual(f.limits, []time.Duration{2 * time.Second}) {
		t.Fatalf("measured %d times, probe limits %v", f.measured, f.limits)
	}
	f.set(func() { f.dead = down("nl") })
	if err := s.Check(context.Background()); err != nil || f.now() != "de" {
		t.Fatalf("remembered order was not kept for switching: err %v, running %q", err, f.now())
	}
}

func TestRememberedBestIsGoneThenMeasure(t *testing.T) {
	f := &fake{delays: map[string]time.Duration{"de": ms(80), "us": ms(60)}, dead: down("nl")}
	var got [][]supervisor.Measurement
	s, tr := setup(f, Options{OnMeasure: func(_ []Candidate, m []supervisor.Measurement) { got = append(got, m) }})
	list := known(cands("de", "nl", "us"), map[string]time.Duration{"de": ms(80), "nl": ms(40)})
	if err := s.Connect(context.Background(), list, ""); err != nil {
		t.Fatal(err)
	}
	same(t, tr.states(), []string{"connecting:nl", "measuring", "connecting:us", "connected:us"})
	if f.stops() != 1 || f.measured != 1 {
		t.Fatalf("stops %d, measured %d", f.stops(), f.measured)
	}
	if len(got) != 1 || len(got[0]) != 3 || !got[0][0].OK() || got[0][1].OK() || got[0][2].Delay != ms(60) {
		t.Fatalf("reported measurements = %+v", got)
	}
}

func TestNoMeasuringThroughTheTunnel(t *testing.T) {
	for _, strict := range []bool{false, true} {
		f := &fake{delays: speeds(80, 40, 120), dead: down("nl")}
		s, _ := setup(f, Options{Strict: strict})
		list := known(cands("de", "nl", "us"), map[string]time.Duration{"de": ms(80), "nl": ms(40)})
		if !strict {
			if err := s.Connect(context.Background(), list, "us"); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.Connect(context.Background(), list, ""); err != nil || f.now() != "de" {
			t.Fatalf("strict %v: err %v, running %q", strict, err, f.now())
		}
		if f.measured != 0 || f.stops() != 0 {
			t.Fatalf("strict %v: measured %d, stops %d", strict, f.measured, f.stops())
		}
	}
	f := &fake{delays: speeds(80, 40, 120)}
	s, _ := setup(f, Options{})
	if err := s.Connect(context.Background(), cands("us", "de", "nl"), "nl"); err != nil {
		t.Fatal(err)
	}
	f.set(func() { f.dead = down("us") })
	if err := s.Connect(context.Background(), cands("us", "de", "nl"), ""); err != nil || f.now() != "de" {
		t.Fatalf("without remembered delays: err %v, running %q", err, f.now())
	}
	if f.measured != 0 || f.stops() != 0 {
		t.Fatalf("measured %d, stops %d", f.measured, f.stops())
	}
}

func TestBypassMeasuresWhileConnected(t *testing.T) {
	f := &fake{delays: speeds(80, 40, 120)}
	s, tr := setup(f, Options{Bypass: true})
	if err := s.Connect(context.Background(), cands("de", "nl", "us"), "us"); err != nil {
		t.Fatal(err)
	}
	if err := s.Connect(context.Background(), cands("de", "nl", "us"), ""); err != nil || f.now() != "nl" {
		t.Fatalf("err %v, running %q", err, f.now())
	}
	if f.measured != 1 || f.stops() != 0 {
		t.Fatalf("measured %d, stops %d", f.measured, f.stops())
	}
	same(t, tr.states()[2:], []string{"measuring", "connecting:nl", "connected:nl"})
}
