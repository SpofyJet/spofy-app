package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/engine"
	"github.com/SpofyJet/spofy-app/core/internal/session"
	"github.com/SpofyJet/spofy-app/core/internal/store"
	"github.com/SpofyJet/spofy-app/core/internal/sub"
	"github.com/SpofyJet/spofy-app/core/internal/supervisor"
	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
)

const (
	Protocol = 1
	Auto     = "auto"
)

const (
	EvConnection   = "connection"
	EvSubscription = "subscription"
	EvServers      = "servers"
	EvSettings     = "settings"
)

const (
	KeyKillSwitch  = "killSwitch"
	KeyAutoConnect = "autoConnect"
	KeyApps        = "apps"
)

const (
	CodeNoSubscription engine.Code = "no_subscription"
	CodeBadURL         engine.Code = "sub_bad_url"
	CodeUnreachable    engine.Code = "sub_unreachable"
	CodeStatus         engine.Code = "sub_status"
	CodeFormat         engine.Code = "sub_format"
	CodeStore          engine.Code = "store_failed"
	CodeConnected      engine.Code = "connected"
	CodeValue          engine.Code = "bad_value"
)

type Tunnel interface {
	session.Tunnel
	Traffic(tag string) (engine.Traffic, error)
}

type Config struct {
	Store    *store.Store
	Tunnel   Tunnel
	Device   sub.Device
	HTTP     *http.Client
	Run      xcfg.Options
	Tier     func(sub.Server) int
	Now      func() time.Time
	Version  string
	Fresh    time.Duration
	Bypass   bool
	Strict   bool
	URL      string
	Timeout  time.Duration
	Retries  []time.Duration
	Parallel int
	Watch    time.Duration
	Quiet    time.Duration
	Renew    time.Duration
	Resample time.Duration
	Settle   time.Duration
	Retune   func() (bool, error)
	Keep     func(on bool)
	Held     bool
}

type Apps struct {
	Mode string   `json:"mode"`
	List []string `json:"list"`
}

const (
	AppsAll    = "all"
	AppsExcept = "except"
	AppsOnly   = "only"
	appsLimit  = 512
)

func apps(value json.RawMessage) (*Apps, bool) {
	if len(value) == 0 || string(value) == "null" {
		return nil, true
	}
	var v Apps
	dec := json.NewDecoder(bytes.NewReader(value))
	dec.DisallowUnknownFields()
	if dec.Decode(&v) != nil || (v.Mode != AppsAll && v.Mode != AppsExcept && v.Mode != AppsOnly) || len(v.List) > appsLimit {
		return nil, false
	}
	seen := map[string]bool{}
	list := []string{}
	for _, name := range v.List {
		if name == "" || len(name) > 255 || strings.Trim(name, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._") != "" {
			return nil, false
		}
		if !seen[name] {
			seen[name] = true
			list = append(list, name)
		}
	}
	slices.Sort(list)
	if len(list) == 0 {
		return nil, v.Mode != AppsOnly
	}
	return &Apps{Mode: v.Mode, List: list}, true
}

func (a *App) Apps() *Apps {
	v, _ := apps(a.cfg.Store.Get().Settings[KeyApps])
	if v == nil || v.Mode == AppsAll {
		return nil
	}
	return v
}

func sameApps(a, b *Apps) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Mode == b.Mode && slices.Equal(a.List, b.List)
}

type Connection struct {
	State   session.State `json:"state"`
	Choice  string        `json:"choice"`
	Server  string        `json:"server"`
	DelayMs int64         `json:"delayMs"`
	Code    engine.Code   `json:"code"`
	Error   string        `json:"error"`
}

type Server struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Label      string `json:"label"`
	Country    string `json:"country"`
	Protocol   string `json:"protocol"`
	Tier       int    `json:"tier"`
	DelayMs    int64  `json:"delayMs"`
	MeasuredAt int64  `json:"measuredAt"`
}

type Subscription struct {
	Title       string   `json:"title"`
	Upload      int64    `json:"upload"`
	Download    int64    `json:"download"`
	Total       int64    `json:"total"`
	Expire      int64    `json:"expire"`
	SupportURL  string   `json:"supportUrl"`
	WebPageURL  string   `json:"webPageUrl"`
	Announce    string   `json:"announce"`
	FetchedAt   int64    `json:"fetchedAt"`
	UpdateEvery int64    `json:"updateEvery"`
	Stop        Stop     `json:"stop"`
	Notes       []string `json:"notes"`
}

type Stop string

const (
	StopEnded   Stop = "ended"
	StopSpent   Stop = "spent"
	StopDevices Stop = "devices"
	StopOther   Stop = "other"
)

func (s Stop) Code() engine.Code {
	return engine.Code("sub_" + string(s))
}

func stop(p *sub.Profile, now time.Time) Stop {
	switch u := p.Usage; {
	case len(p.Servers) > 0:
		return ""
	case p.Flags["hwid-max-devices-reached"] == "true":
		return StopDevices
	case !u.Expire.IsZero() && !u.Expire.After(now):
		return StopEnded
	case u.Total > 0 && u.Upload+u.Download >= u.Total:
		return StopSpent
	}
	return StopOther
}

type Entry struct {
	At     int64         `json:"at"`
	State  session.State `json:"state"`
	Server string        `json:"server"`
	Code   engine.Code   `json:"code"`
}

type Report struct {
	OS        string  `json:"os"`
	OSVersion string  `json:"osVersion"`
	Model     string  `json:"model"`
	Device    string  `json:"device"`
	StartedAt int64   `json:"startedAt"`
	Journal   []Entry `json:"journal"`
}

const journalSize = 40

type Snapshot struct {
	Version      string                     `json:"version"`
	Protocol     int                        `json:"protocol"`
	Connection   Connection                 `json:"connection"`
	Subscription *Subscription              `json:"subscription"`
	Servers      []Server                   `json:"servers"`
	Settings     map[string]json.RawMessage `json:"settings"`
}

type Event struct {
	Ev           string
	Connection   Connection
	Subscription *Subscription
	Servers      []Server
	Settings     map[string]json.RawMessage
}

func (e Event) MarshalJSON() ([]byte, error) {
	var part any
	switch e.Ev {
	case EvConnection:
		part = e.Connection
	case EvSubscription:
		part = e.Subscription
	case EvServers:
		part = e.Servers
	case EvSettings:
		part = e.Settings
	default:
		return nil, errors.New("app: unknown event")
	}
	body, err := json.Marshal(part)
	if err != nil {
		return nil, err
	}
	return []byte(`{"ev":"` + e.Ev + `","` + e.Ev + `":` + string(body) + `}`), nil
}

type App struct {
	cfg  Config
	sess *session.Session

	ctx    context.Context
	cancel context.CancelFunc
	online atomic.Bool
	due    atomic.Int64

	mu      sync.Mutex
	conn    Connection
	hearers map[int]func(Event)
	next    int
	hush    context.CancelFunc
	sampled time.Time
	closed  bool
	tasks   sync.WaitGroup
	journal []Entry
	started int64
	renews  bool
	tried   time.Time
}

func New(cfg Config) *App {
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Tier == nil {
		cfg.Tier = func(sub.Server) int { return 0 }
	}
	if cfg.Fresh == 0 {
		cfg.Fresh = 6 * time.Hour
	}
	if cfg.Renew == 0 {
		cfg.Renew = 15 * time.Minute
	}
	if cfg.Resample == 0 {
		cfg.Resample = time.Hour
	}
	if cfg.Settle == 0 {
		cfg.Settle = 10 * time.Second
	}
	a := &App{cfg: cfg, hearers: map[int]func(Event){}}
	a.started = a.now().Unix()
	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.online.Store(true)
	a.plan(cfg.Store.Get(), false)
	a.conn = Connection{State: session.Idle, Choice: a.choice(cfg.Store.Get())}
	a.sess = session.New(session.Options{
		Tunnel:    cfg.Tunnel,
		OnEvent:   a.onState,
		OnMeasure: a.onMeasure,
		Online:    a.online.Load,
		Strict:    cfg.Strict || on(cfg.Store.Get(), KeyKillSwitch),
		Bypass:    cfg.Bypass,
		URL:       cfg.URL,
		Timeout:   cfg.Timeout,
		Retries:   cfg.Retries,
		Parallel:  cfg.Parallel,
	})
	if cfg.Watch > 0 {
		a.spawn(a.watch)
	}
	return a
}

func (a *App) spawn(work func()) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.tasks.Add(1)
	a.mu.Unlock()
	go func() {
		defer a.tasks.Done()
		work()
	}()
}

func (a *App) watch() {
	tick := time.NewTicker(a.cfg.Watch)
	defer tick.Stop()
	var w stall
	var was Connection
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-tick.C:
		}
		a.mu.Lock()
		now := a.conn
		a.mu.Unlock()
		if now.State != was.State || now.Server != was.Server {
			w, was = stall{}, now
		}
		if now.State != session.Connected && now.State != session.Stalled {
			continue
		}
		if at := a.due.Load(); at != 0 && a.cfg.Now().UnixNano() >= at {
			a.spawn(func() { a.freshen(false) })
		}
		if w.step(a.flow(), a.cfg.Quiet, time.Now()) {
			a.sess.Suspect(a.ctx)
		}
	}
}

func (a *App) Listen(hear func(Event)) (stop func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	id := a.next
	a.next++
	a.hearers[id] = hear
	return func() {
		a.mu.Lock()
		delete(a.hearers, id)
		a.mu.Unlock()
	}
}

func (a *App) Snapshot() Snapshot {
	st := a.cfg.Store.Get()
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	return Snapshot{
		Version:      a.cfg.Version,
		Protocol:     Protocol,
		Connection:   conn,
		Subscription: subscription(st, a.now()),
		Servers:      a.servers(st),
		Settings:     settings(st),
	}
}

func (a *App) Subscribe(ctx context.Context, address string) error {
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fail(CodeBadURL, "the address must start with https://")
	}
	p, err := sub.Fetch(ctx, a.cfg.HTTP, u.String(), a.cfg.Device, "")
	if errors.Is(err, sub.ErrFormat) {
		named := u.JoinPath("json")
		if again, failed := sub.Fetch(ctx, a.cfg.HTTP, named.String(), a.cfg.Device, ""); failed == nil {
			u, p, err = named, again, nil
		}
	}
	if err != nil {
		return fetchError(err)
	}
	return a.adopt(u.String(), p)
}

func (a *App) Refresh(ctx context.Context) (bool, error) {
	st := a.cfg.Store.Get()
	if st.URL == "" || st.Profile == nil {
		return false, fail(CodeNoSubscription, "no subscription")
	}
	p, err := sub.Fetch(ctx, a.cfg.HTTP, st.URL, a.cfg.Device, st.Profile.ETag)
	if errors.Is(err, sub.ErrNotModified) {
		err = a.save(func(s *store.State) {
			if s.Profile == nil {
				return
			}
			s.FetchedAt = a.now()
			if p != nil {
				s.Profile.Usage = p.Usage
			}
		})
		if err == nil {
			now := a.cfg.Store.Get()
			a.plan(now, false)
			a.emit(Event{Ev: EvSubscription, Subscription: subscription(now, a.now())})
		}
		return false, err
	}
	if err != nil {
		a.plan(st, true)
		return false, fetchError(err)
	}
	return true, a.adopt(st.URL, p)
}

const renewDefault = 12 * time.Hour

func (a *App) plan(st store.State, failed bool) {
	if st.URL == "" || st.Profile == nil || a.cfg.Renew < 0 {
		a.due.Store(0)
		return
	}
	every := st.Profile.UpdateEvery
	if every <= 0 {
		every = renewDefault
	}
	every = max(every, a.cfg.Renew)
	at := st.FetchedAt.Add(every)
	if failed {
		at = a.cfg.Now().Add(max(every/4, a.cfg.Renew))
	}
	a.due.Store(at.UnixNano())
}

func (a *App) freshen(forced bool) {
	now := a.cfg.Now()
	at := a.due.Load()
	if at == 0 || !a.online.Load() || (!forced && now.UnixNano() < at) {
		return
	}
	a.mu.Lock()
	busy := a.renews || now.Sub(a.tried) < min(a.cfg.Renew, time.Minute)
	if !busy {
		a.renews, a.tried = true, now
	}
	a.mu.Unlock()
	if busy {
		return
	}
	ctx, cancel := context.WithTimeout(a.ctx, time.Minute)
	changed, _ := a.Refresh(ctx)
	cancel()
	a.mu.Lock()
	a.renews = false
	a.mu.Unlock()
	if changed && forced && a.sess.State() == session.Failed {
		a.renew()
	}
}

func (a *App) renew() {
	st := a.cfg.Store.Get()
	if st.Profile == nil {
		return
	}
	pinned := ""
	if has(st.Profile, st.Server) {
		pinned = st.Server
	}
	if cands := a.candidates(st); len(cands) > 0 {
		a.sess.Renew(a.ctx, cands, pinned)
	}
}

func (a *App) Unsubscribe() error {
	a.sess.Disconnect()
	err := a.save(func(s *store.State) {
		s.URL, s.Profile, s.Server, s.Delays, s.FetchedAt = "", nil, "", nil, time.Time{}
	})
	if err != nil {
		return err
	}
	a.plan(a.cfg.Store.Get(), false)
	a.setChoice(Auto)
	a.emit(Event{Ev: EvSubscription})
	a.emit(Event{Ev: EvServers, Servers: []Server{}})
	return nil
}

func (a *App) Connect(server string) error {
	st := a.cfg.Store.Get()
	if st.Profile == nil {
		return fail(CodeNoSubscription, "no subscription")
	}
	if len(st.Profile.Servers) == 0 {
		if a.online.Load() {
			ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
			a.Refresh(ctx)
			cancel()
			st = a.cfg.Store.Get()
		}
		if st.Profile == nil {
			return fail(CodeNoSubscription, "no subscription")
		}
		if why := stop(st.Profile, a.now()); why != "" {
			return fail(why.Code(), "the subscription gives no servers")
		}
	}
	if server == "" {
		server = a.choice(st)
	}
	pinned := ""
	if server != Auto {
		pinned = server
		if !has(st.Profile, pinned) {
			return fail(session.CodeNotFound, "no such server")
		}
	}
	cands := a.candidates(st)
	if len(cands) == 0 {
		return fail(engine.CodeConfig, "no usable server in the subscription")
	}
	if err := a.save(func(s *store.State) { s.Server = pinned }); err != nil {
		return err
	}
	a.setChoice(server)
	return outcome(a.sess.Connect(a.ctx, cands, pinned))
}

func (a *App) Disconnect() error {
	return a.sess.Disconnect()
}

func (a *App) Check() error {
	a.spawn(func() { a.freshen(false) })
	a.spawn(a.resample)
	return outcome(a.sess.Check(a.ctx))
}

func (a *App) resample() {
	if !a.cfg.Bypass || a.cfg.Resample < 0 || !a.online.Load() || a.sess.State() != session.Connected {
		return
	}
	st := a.cfg.Store.Get()
	if st.Profile == nil || has(st.Profile, st.Server) {
		return
	}
	var oldest time.Time
	for _, s := range st.Profile.Servers {
		if d, ok := st.Delays[s.ID]; ok && (oldest.IsZero() || d.At.Before(oldest)) {
			oldest = d.At
		}
	}
	if oldest.IsZero() || a.cfg.Now().Sub(oldest) < a.cfg.Resample {
		return
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	now := a.cfg.Now()
	a.mu.Lock()
	busy := a.hush != nil || (!a.sampled.IsZero() && now.Sub(a.sampled) < min(a.cfg.Resample, 5*time.Minute))
	if !busy {
		a.hush, a.sampled = cancel, now
	}
	a.mu.Unlock()
	if busy {
		return
	}
	cands := a.candidates(st)
	measured := a.sess.MeasureQuietly(ctx, cands)
	stopped := ctx.Err() != nil
	a.mu.Lock()
	a.hush = nil
	a.mu.Unlock()
	if stopped {
		return
	}
	a.onMeasure(cands, measured)
	a.sess.Reorder(cands, measured)
}

func (a *App) SetOnline(online bool) error {
	a.online.Store(online)
	a.sess.Network(online)
	if !online {
		return nil
	}
	return a.Check()
}

func (a *App) Measure(ctx context.Context) error {
	st := a.cfg.Store.Get()
	if st.Profile == nil {
		return fail(CodeNoSubscription, "no subscription")
	}
	if !a.cfg.Bypass && a.sess.State() != session.Idle && a.sess.State() != session.Failed {
		return fail(CodeConnected, "servers cannot be measured while the tunnel is up")
	}
	a.mu.Lock()
	hush := a.hush
	a.mu.Unlock()
	if hush != nil {
		hush()
	}
	cands := a.candidates(st)
	measured := a.sess.MeasureLive(ctx, cands, 150*time.Millisecond, func(part []supervisor.Measurement) { a.onMeasure(cands, part) })
	if ctx.Err() != nil {
		return fail(supervisor.CodeCanceled, "canceled")
	}
	a.onMeasure(cands, measured)
	return nil
}

func (a *App) Set(key string, value json.RawMessage) error {
	limit := 4096
	if key == KeyApps {
		limit = 64 << 10
	}
	if key == "" || len(key) > 64 || len(value) > limit || (len(value) > 0 && !json.Valid(value)) {
		return fail(CodeValue, "bad setting")
	}
	if v := string(value); (key == KeyKillSwitch || key == KeyAutoConnect) && v != "" && v != "null" && v != "true" && v != "false" {
		return fail(CodeValue, "bad setting")
	}
	var applied *Apps
	if key == KeyApps {
		chosen, ok := apps(value)
		if !ok {
			return fail(CodeValue, "bad setting")
		}
		value = nil
		if chosen != nil {
			value, _ = json.Marshal(chosen)
		}
		applied = a.Apps()
	}
	err := a.save(func(s *store.State) {
		if len(value) == 0 || string(value) == "null" {
			delete(s.Settings, key)
			return
		}
		if s.Settings == nil {
			s.Settings = map[string]json.RawMessage{}
		}
		s.Settings[key] = value
	})
	if err != nil {
		return err
	}
	if key == KeyKillSwitch {
		strict := a.cfg.Strict || on(a.cfg.Store.Get(), KeyKillSwitch)
		a.sess.SetStrict(strict)
		if a.cfg.Keep != nil {
			a.cfg.Keep(strict)
		}
	}
	a.emit(Event{Ev: EvSettings, Settings: settings(a.cfg.Store.Get())})
	if key == KeyApps && a.cfg.Retune != nil && a.sess.State() != session.Idle && !sameApps(applied, a.Apps()) {
		a.spawn(func() {
			if again, err := a.cfg.Retune(); again && err == nil {
				a.renew()
			}
		})
	}
	return nil
}

func (a *App) Resume() error {
	st := a.cfg.Store.Get()
	if a.cfg.Keep != nil {
		a.cfg.Keep(a.cfg.Strict || on(st, KeyKillSwitch))
	}
	if st.Profile == nil || !(on(st, KeyAutoConnect) || a.cfg.Held) {
		if a.cfg.Held {
			a.cfg.Tunnel.Stop()
		}
		return nil
	}
	err := a.Connect("")
	if err != nil && a.cfg.Held && a.sess.State() == session.Idle {
		a.cfg.Tunnel.Stop()
	}
	return err
}

func on(st store.State, key string) bool {
	return string(st.Settings[key]) == "true"
}

func (a *App) Report() Report {
	a.mu.Lock()
	defer a.mu.Unlock()
	d := a.cfg.Device
	return Report{OS: d.OS, OSVersion: d.OSVersion, Model: d.Model, Device: d.HWID, StartedAt: a.started, Journal: append([]Entry{}, a.journal...)}
}

func (a *App) Traffic() engine.Traffic {
	t := a.flow()
	return engine.Traffic{Up: t.Up, Down: t.Down}
}

func (a *App) flow() engine.Traffic {
	t, _ := a.cfg.Tunnel.Traffic(xcfg.TagTun)
	return t
}

func (a *App) Close() {
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()
	a.cancel()
	a.sess.Disconnect()
	a.tasks.Wait()
}

func (a *App) adopt(address string, p *sub.Profile) error {
	err := a.save(func(s *store.State) {
		s.URL, s.Profile, s.FetchedAt = address, p, a.now()
		for id := range s.Delays {
			if !has(p, id) {
				delete(s.Delays, id)
			}
		}
		if s.Server != "" && !has(p, s.Server) {
			s.Server = ""
		}
	})
	if err != nil {
		return err
	}
	st := a.cfg.Store.Get()
	a.plan(st, false)
	a.setChoice(a.choice(st))
	a.emit(Event{Ev: EvSubscription, Subscription: subscription(st, a.now())})
	a.emit(Event{Ev: EvServers, Servers: a.servers(st)})
	switch {
	case len(p.Servers) == 0:
		a.sess.Disconnect()
	case a.sess.State() != session.Idle && a.sess.Rebase(a.candidates(st)):
		a.spawn(a.renew)
	}
	return nil
}

func (a *App) save(change func(*store.State)) error {
	if err := a.cfg.Store.Update(change); err != nil {
		return fail(CodeStore, "state was not saved")
	}
	return nil
}

func (a *App) now() time.Time {
	return a.cfg.Now().UTC().Truncate(time.Second)
}

func (a *App) choice(st store.State) string {
	if st.Server == "" {
		return Auto
	}
	return st.Server
}

func (a *App) setChoice(choice string) {
	a.mu.Lock()
	changed := a.conn.Choice != choice
	a.conn.Choice = choice
	conn := a.conn
	a.mu.Unlock()
	if changed {
		a.emit(Event{Ev: EvConnection, Connection: conn})
	}
}

func (a *App) onState(e session.Event) {
	a.mu.Lock()
	conn := Connection{State: e.State, Choice: a.conn.Choice, Server: e.Server, DelayMs: e.Delay.Milliseconds(), Code: e.Code, Error: e.Error}
	same := conn == a.conn
	arrived := e.State == session.Connected && a.conn.State != session.Connected
	if conn.State != a.conn.State || conn.Server != a.conn.Server || conn.Code != a.conn.Code {
		a.journal = append(a.journal, Entry{At: a.now().Unix(), State: conn.State, Server: conn.Server, Code: conn.Code})
		a.journal = a.journal[max(len(a.journal)-journalSize, 0):]
	}
	a.conn = conn
	hush := a.hush
	a.mu.Unlock()
	if !same {
		a.emit(Event{Ev: EvConnection, Connection: conn})
	}
	if e.State != session.Connected && hush != nil {
		hush()
	}
	if arrived {
		time.AfterFunc(a.cfg.Settle, func() { a.spawn(a.resample) })
	}
	switch {
	case e.State == session.Connected:
		a.spawn(func() { a.freshen(false) })
	case e.State == session.Failed && (e.Code == session.CodeNoServer || e.Code == session.CodeUnreachable):
		a.spawn(func() { a.freshen(true) })
	}
}

func (a *App) onMeasure(cands []session.Candidate, measured []supervisor.Measurement) {
	at := a.now()
	err := a.cfg.Store.Update(func(s *store.State) {
		if s.Delays == nil {
			s.Delays = map[string]store.Delay{}
		}
		for i, m := range measured {
			switch {
			case m.OK():
				s.Delays[cands[i].ID] = store.Delay{Ms: max(m.Delay.Milliseconds(), 1), At: at}
			case m.Code != supervisor.CodeCanceled:
				delete(s.Delays, cands[i].ID)
			}
		}
	})
	if err == nil {
		a.emit(Event{Ev: EvServers, Servers: a.servers(a.cfg.Store.Get())})
	}
}

func (a *App) emit(e Event) {
	a.mu.Lock()
	hearers := make([]func(Event), 0, len(a.hearers))
	for id := 0; id < a.next; id++ {
		if h, ok := a.hearers[id]; ok {
			hearers = append(hearers, h)
		}
	}
	a.mu.Unlock()
	for _, h := range hearers {
		h(e)
	}
}

func (a *App) servers(st store.State) []Server {
	out := []Server{}
	if st.Profile == nil {
		return out
	}
	for _, s := range st.Profile.Servers {
		v := Server{ID: s.ID, Name: s.Name, Label: s.Label, Country: s.Country, Protocol: s.Protocol, Tier: a.cfg.Tier(s)}
		if d, ok := st.Delays[s.ID]; ok {
			v.DelayMs, v.MeasuredAt = d.Ms, d.At.Unix()
		}
		out = append(out, v)
	}
	return out
}

func (a *App) candidates(st store.State) []session.Candidate {
	var out []session.Candidate
	now := a.cfg.Now()
	for _, s := range st.Profile.Servers {
		run, err := xcfg.Build(s.Config, a.cfg.Run)
		if err != nil {
			continue
		}
		probe, err := xcfg.Build(s.Config, xcfg.Options{Mode: xcfg.ModeProbe, LogLevel: "none"})
		if err != nil {
			continue
		}
		c := session.Candidate{ID: s.ID, Tier: a.cfg.Tier(s), Run: run, Probe: probe, Tag: xcfg.MainTag(s.Config)}
		if d, ok := st.Delays[s.ID]; ok && d.Ms > 0 && now.Sub(d.At) < a.cfg.Fresh {
			c.Known = time.Duration(d.Ms) * time.Millisecond
		}
		out = append(out, c)
	}
	return out
}

func subscription(st store.State, now time.Time) *Subscription {
	p := st.Profile
	if p == nil {
		return nil
	}
	s := &Subscription{
		Title:       p.Title,
		Upload:      p.Usage.Upload,
		Download:    p.Usage.Download,
		Total:       p.Usage.Total,
		SupportURL:  p.SupportURL,
		WebPageURL:  p.WebPageURL,
		Announce:    p.Announce,
		UpdateEvery: int64(p.UpdateEvery / time.Second),
		Stop:        stop(p, now),
		Notes:       append([]string{}, p.Notes...),
	}
	if !p.Usage.Expire.IsZero() {
		s.Expire = p.Usage.Expire.Unix()
	}
	if !st.FetchedAt.IsZero() {
		s.FetchedAt = st.FetchedAt.Unix()
	}
	return s
}

func settings(st store.State) map[string]json.RawMessage {
	if st.Settings == nil {
		return map[string]json.RawMessage{}
	}
	return st.Settings
}

func has(p *sub.Profile, id string) bool {
	for _, s := range p.Servers {
		if s.ID == id {
			return true
		}
	}
	return false
}

func outcome(err error) error {
	var e *engine.Error
	switch {
	case err == nil:
		return nil
	case errors.As(err, &e):
		return err
	case errors.Is(err, context.Canceled), errors.Is(err, supervisor.ErrSuperseded):
		return fail(supervisor.CodeCanceled, "canceled")
	}
	return fail(supervisor.CodeUnavailable, err.Error())
}

func fail(code engine.Code, text string) error {
	return &engine.Error{Code: code, Err: errors.New(text)}
}

func fetchError(err error) error {
	var status *sub.StatusError
	var ue *url.Error
	switch {
	case errors.As(err, &status):
		return fail(CodeStatus, status.Error())
	case errors.Is(err, sub.ErrFormat):
		return fail(CodeFormat, "the address did not return a subscription")
	case errors.As(err, &ue):
		return fail(CodeUnreachable, ue.Err.Error())
	}
	return fail(CodeUnreachable, "the subscription server did not answer")
}
