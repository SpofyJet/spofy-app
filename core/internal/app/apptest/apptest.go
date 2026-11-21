package apptest

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/engine"
	"github.com/SpofyJet/spofy-app/core/internal/sub"
	"github.com/SpofyJet/spofy-app/core/internal/supervisor"
)

const Address = "https://sub.example.test/sub/TESTTOKEN0123456789"

func Fixture(t testing.TB) ([]byte, http.Header) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "sub")
	body, err := os.ReadFile(filepath.Join(dir, "sub.json"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "headers.json"))
	if err != nil {
		t.Fatal(err)
	}
	var plain map[string]string
	if err := json.Unmarshal(raw, &plain); err != nil {
		t.Fatal(err)
	}
	h := http.Header{}
	for k, v := range plain {
		h.Set(k, v)
	}
	return body, h
}

type Panel struct {
	srv *httptest.Server

	mu      sync.Mutex
	body    []byte
	header  http.Header
	status  int
	hits    int
	last    http.Header
	usage   string
	holding chan struct{}
	links   []byte
	noJSON  bool
	paths   []string
}

func NewPanel(t testing.TB) *Panel {
	t.Helper()
	body, h := Fixture(t)
	p := &Panel{body: body, header: h}
	p.srv = httptest.NewTLSServer(http.HandlerFunc(p.serve))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *Panel) serve(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.hits++
	p.last = r.Header.Clone()
	p.paths = append(p.paths, r.URL.Path)
	body, header, status, usage, holding := p.body, p.header.Clone(), p.status, p.usage, p.holding
	named := strings.HasSuffix(r.URL.Path, "/json")
	if p.links != nil && !named {
		body = p.links
		header.Set("Content-Type", "text/plain; charset=utf-8")
	}
	if p.noJSON && named && status == 0 {
		status = http.StatusNotFound
	}
	p.mu.Unlock()
	if holding != nil {
		select {
		case <-holding:
		case <-r.Context().Done():
			return
		}
	}
	if usage != "" {
		header.Set("Subscription-Userinfo", usage)
	}
	switch {
	case status != 0:
		w.WriteHeader(status)
	case r.Header.Get("If-None-Match") != "" && r.Header.Get("If-None-Match") == header.Get("Etag"):
		w.Header().Set("Subscription-Userinfo", header.Get("Subscription-Userinfo"))
		w.WriteHeader(http.StatusNotModified)
	default:
		for k, v := range header {
			w.Header()[k] = v
		}
		w.Write(body)
	}
}

func (p *Panel) Client() *http.Client {
	addr := p.srv.Listener.Addr().String()
	pool := p.srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "example.com"},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			},
		},
	}
}

func (p *Panel) Set(change func(body *[]byte, header http.Header)) {
	p.mu.Lock()
	change(&p.body, p.header)
	p.mu.Unlock()
}

func (p *Panel) Status(code int) {
	p.mu.Lock()
	p.status = code
	p.mu.Unlock()
}

func (p *Panel) Usage(v string) {
	p.mu.Lock()
	p.usage = v
	p.mu.Unlock()
}

func (p *Panel) Hold() (release func()) {
	ch := make(chan struct{})
	p.mu.Lock()
	p.holding = ch
	p.mu.Unlock()
	return func() {
		p.mu.Lock()
		p.holding = nil
		p.mu.Unlock()
		close(ch)
	}
}

func (p *Panel) Links(list string) {
	p.mu.Lock()
	p.links = []byte(list)
	p.mu.Unlock()
}

func (p *Panel) WithoutJSON() {
	p.mu.Lock()
	p.noJSON = true
	p.mu.Unlock()
}

func (p *Panel) Paths() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string{}, p.paths...)
}

func (p *Panel) Hits() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hits
}

func (p *Panel) Last() http.Header {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.last
}

func (p *Panel) Close() {
	p.srv.Close()
}

type Clock struct {
	mu  sync.Mutex
	now time.Time
}

func NewClock() *Clock {
	return &Clock{now: time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)}
}

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

type Tunnel struct {
	mu       sync.Mutex
	ids      map[string]string
	delays   map[string]time.Duration
	dead     map[string]bool
	running  string
	starts   []string
	stops    int
	measured int
	workers  []int
	traffic  engine.Traffic
	gate     chan struct{}
	slow     chan struct{}
}

func NewTunnel(t testing.TB) *Tunnel {
	t.Helper()
	body, h := Fixture(t)
	p, err := sub.Parse(body, h)
	if err != nil {
		t.Fatal(err)
	}
	tn := &Tunnel{ids: map[string]string{}, delays: map[string]time.Duration{}, dead: map[string]bool{}}
	for _, s := range p.Servers {
		k := key(s.Config)
		if other, ok := tn.ids[k]; ok {
			t.Fatalf("fixture servers %s and %s share the first outbound", other, s.ID)
		}
		tn.ids[k] = s.ID
	}
	return tn
}

func key(cfg []byte) string {
	var c struct {
		Outbounds []any `json:"outbounds"`
	}
	if json.Unmarshal(cfg, &c) != nil || len(c.Outbounds) == 0 {
		return ""
	}
	b, _ := json.Marshal(c.Outbounds[0])
	sum := sha256.Sum256(b)
	return string(sum[:])
}

func (t *Tunnel) Know(servers []sub.Server) {
	t.mu.Lock()
	for _, s := range servers {
		t.ids[key(s.Config)] = s.ID
	}
	t.mu.Unlock()
}

func (t *Tunnel) Answer(delays map[string]time.Duration) {
	t.mu.Lock()
	t.delays = delays
	t.mu.Unlock()
}

func (t *Tunnel) Kill(ids ...string) {
	t.mu.Lock()
	t.dead = map[string]bool{}
	for _, id := range ids {
		t.dead[id] = true
	}
	t.mu.Unlock()
}

func (t *Tunnel) Count(up, down int64) {
	t.mu.Lock()
	t.traffic = engine.Traffic{Up: up, Down: down, Back: down}
	t.mu.Unlock()
}

func (t *Tunnel) Running() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

func (t *Tunnel) Stops() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stops
}

func (t *Tunnel) Starts() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string{}, t.starts...)
}

func (t *Tunnel) Measured() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.measured
}

func (t *Tunnel) Workers() []int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]int{}, t.workers...)
}

func (t *Tunnel) HoldMeasure() (release func()) {
	slow := make(chan struct{})
	t.mu.Lock()
	t.slow = slow
	t.mu.Unlock()
	return func() {
		t.mu.Lock()
		t.slow = nil
		t.mu.Unlock()
		close(slow)
	}
}

func (t *Tunnel) Start(cfg []byte) (time.Duration, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	id, ok := t.ids[key(cfg)]
	if !ok {
		t.running = ""
		return 0, &engine.Error{Code: engine.CodeConfig, Err: errors.New("unknown config")}
	}
	t.running = id
	t.starts = append(t.starts, id)
	return time.Millisecond, nil
}

func (t *Tunnel) Stop() error {
	t.mu.Lock()
	t.running = ""
	t.stops++
	t.mu.Unlock()
	return nil
}

func (t *Tunnel) Hold() (release func()) {
	gate := make(chan struct{})
	t.mu.Lock()
	t.gate = gate
	t.mu.Unlock()
	return func() {
		t.mu.Lock()
		t.gate = nil
		t.mu.Unlock()
		close(gate)
	}
}

func (t *Tunnel) Probe(ctx context.Context, _, _ string, _ time.Duration) (time.Duration, error) {
	t.mu.Lock()
	gate := t.gate
	t.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	switch {
	case ctx.Err() != nil:
		return 0, &engine.Error{Code: supervisor.CodeCanceled, Err: ctx.Err()}
	case t.running == "":
		return 0, &engine.Error{Code: supervisor.CodeUnavailable, Err: errors.New("engine is not running")}
	case t.dead[t.running] || t.delays[t.running] == 0:
		return 0, &engine.Error{Code: engine.CodeProbe, Err: errors.New("no answer")}
	}
	return t.delays[t.running], nil
}

func (t *Tunnel) Measure(ctx context.Context, targets []supervisor.Target, o supervisor.MeasureOptions, each func(int, supervisor.Measurement)) {
	t.mu.Lock()
	t.measured++
	t.workers = append(t.workers, o.Parallel)
	slow := t.slow
	t.mu.Unlock()
	if slow != nil {
		select {
		case <-slow:
		case <-ctx.Done():
		}
	}
	for i, target := range targets {
		if ctx.Err() != nil {
			return
		}
		t.mu.Lock()
		id := t.ids[key(target.Config)]
		delay, dead := t.delays[id], t.dead[id]
		t.mu.Unlock()
		if delay == 0 || dead {
			each(i, supervisor.Measurement{Code: engine.CodeProbe, Error: "no answer"})
			continue
		}
		each(i, supervisor.Measurement{Delay: delay})
	}
}

func (t *Tunnel) Traffic(string) (engine.Traffic, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.traffic, nil
}
