package session

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/engine"
	"github.com/SpofyJet/spofy-app/core/internal/supervisor"
)

type State string

const (
	Idle       State = "idle"
	Measuring  State = "measuring"
	Connecting State = "connecting"
	Connected  State = "connected"
	Switching  State = "switching"
	Stalled    State = "stalled"
	Offline    State = "offline"
	Failed     State = "failed"
)

const (
	CodeNoServer    engine.Code = "no_server"
	CodeUnreachable engine.Code = "server_unreachable"
	CodeNotFound    engine.Code = "server_not_found"
)

type Tunnel interface {
	Start(cfg []byte) (time.Duration, error)
	Stop() error
	Probe(ctx context.Context, tag, url string, timeout time.Duration) (time.Duration, error)
	Measure(ctx context.Context, targets []supervisor.Target, o supervisor.MeasureOptions, each func(int, supervisor.Measurement))
}

type Candidate struct {
	ID    string
	Tier  int
	Run   []byte
	Probe []byte
	Tag   string
	Known time.Duration
}

type Event struct {
	State  State         `json:"state"`
	Server string        `json:"server,omitempty"`
	Delay  time.Duration `json:"delay,omitempty"`
	Code   engine.Code   `json:"code,omitempty"`
	Error  string        `json:"error,omitempty"`
}

type Options struct {
	Tunnel    Tunnel
	OnEvent   func(Event)
	OnMeasure func([]Candidate, []supervisor.Measurement)
	Online    func() bool
	Strict    bool
	Bypass    bool
	URL       string
	Timeout   time.Duration
	Retries   []time.Duration
	Parallel  int
}

type Session struct {
	opts Options
	op   sync.Mutex

	mu      sync.Mutex
	epoch   context.Context
	cancel  context.CancelFunc
	waiting int
	cands   []Candidate
	order   []int
	active  int
	up      bool
	held    bool
	strict  bool
	pinned  bool
	state   State
	last    time.Duration
	halt    context.CancelFunc
	net     atomic.Uint64
}

func New(o Options) *Session {
	if o.Timeout == 0 {
		o.Timeout = 4 * time.Second
	}
	if len(o.Retries) == 0 {
		o.Retries = []time.Duration{0, time.Second}
	}
	if o.Parallel == 0 {
		o.Parallel = 4
	}
	return &Session{opts: o, active: -1, state: Idle, strict: o.Strict}
}

func (s *Session) SetStrict(on bool) {
	s.mu.Lock()
	s.strict = on
	s.mu.Unlock()
}

func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *Session) Measure(ctx context.Context, cands []Candidate) []supervisor.Measurement {
	targets := make([]supervisor.Target, len(cands))
	for i, c := range cands {
		targets[i] = supervisor.Target{Config: c.Probe, Tag: c.Tag}
	}
	out := make([]supervisor.Measurement, len(cands))
	for i := range out {
		out[i].Code = supervisor.CodeCanceled
	}
	var mu sync.Mutex
	s.opts.Tunnel.Measure(ctx, targets, supervisor.MeasureOptions{URL: s.opts.URL, Timeout: s.opts.Timeout, Parallel: s.opts.Parallel}, func(i int, m supervisor.Measurement) {
		mu.Lock()
		out[i] = m
		mu.Unlock()
	})
	return out
}

func (s *Session) Connect(parent context.Context, cands []Candidate, pinned string) error {
	return s.connect(parent, cands, pinned, false)
}

func (s *Session) Renew(parent context.Context, cands []Candidate, pinned string) error {
	return s.connect(parent, cands, pinned, true)
}

func (s *Session) connect(parent context.Context, cands []Candidate, pinned string, renew bool) error {
	var order []int
	if pinned != "" {
		for i, c := range cands {
			if c.ID == pinned {
				order = []int{i}
			}
		}
		if order == nil {
			return &engine.Error{Code: CodeNotFound, Err: errors.New("no such server")}
		}
	}
	epoch := s.preempt()
	s.acquire()
	if renew && s.State() == Idle {
		s.op.Unlock()
		return nil
	}
	ctx, done := s.bind(parent, epoch)
	defer done()
	if pinned != "" {
		s.install(cands, order, true)
		return s.walk(ctx, Connecting, order)
	}
	tried := -1
	if quick := remembered(cands, false); len(quick) > 0 {
		s.install(cands, quick, false)
		tried = quick[0]
		limit := min(max(5*cands[tried].Known, 2*time.Second), s.opts.Timeout)
		if err, ok := s.settle(ctx, s.try(ctx, Connecting, tried, limit, s.opts.Retries[:1])); ok {
			return err
		}
	}
	order, answered, err := s.rank(ctx, cands, tried)
	if err != nil {
		return err
	}
	s.install(cands, order, false)
	if answered {
		return s.walk(ctx, Connecting, order)
	}
	err, ok := s.settle(ctx, s.try(ctx, Connecting, order[0], min(2*time.Second, s.opts.Timeout), s.opts.Retries[:1]))
	if ok {
		return err
	}
	return s.lost(ctx, CodeNoServer, err)
}

func (s *Session) settle(ctx context.Context, err error) (error, bool) {
	if err, ok := settled(ctx, err); ok {
		return err, true
	}
	var e *engine.Error
	if errors.As(err, &e) && (e.Code == supervisor.CodeGuard || e.Code == engine.CodePermission) {
		return s.lost(ctx, e.Code, e.Err), true
	}
	return err, false
}

func settled(ctx context.Context, err error) (error, bool) {
	switch {
	case err == nil:
		return nil, true
	case ctx.Err() != nil:
		return ctx.Err(), true
	case errors.Is(err, supervisor.ErrSuperseded):
		return err, true
	}
	return err, false
}

func (s *Session) install(cands []Candidate, order []int, pinned bool) {
	s.mu.Lock()
	s.cands, s.order, s.pinned, s.active = cands, order, pinned, -1
	s.mu.Unlock()
}

func (s *Session) rank(ctx context.Context, cands []Candidate, tried int) ([]int, bool, error) {
	s.mu.Lock()
	up, held, strict := s.up, s.held, s.strict
	s.mu.Unlock()
	if up && !s.opts.Bypass {
		if held {
			var order []int
			for _, i := range remembered(cands, true) {
				if i != tried {
					order = append(order, i)
				}
			}
			if tried >= 0 {
				order = append(order, tried)
			}
			return order, true, nil
		}
		s.opts.Tunnel.Stop()
		s.mu.Lock()
		s.up, s.active = false, -1
		s.mu.Unlock()
	}
	s.set(Event{State: Measuring})
	measured := s.Measure(ctx, cands)
	if ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	if s.opts.OnMeasure != nil {
		s.opts.OnMeasure(cands, measured)
	}
	results := make([]Result, len(cands))
	for i, m := range measured {
		results[i] = Result{Tier: cands[i].Tier, Delay: m.Delay, OK: m.OK()}
	}
	order := Order(results, -1)
	if len(order) > 0 {
		return order, true, nil
	}
	if strict {
		return remembered(cands, true), false, nil
	}
	return nil, false, s.lost(ctx, CodeNoServer, errors.New("no server answered"))
}

func remembered(cands []Candidate, all bool) []int {
	results := make([]Result, len(cands))
	for i, c := range cands {
		results[i] = Result{Tier: c.Tier, Delay: c.Known, OK: c.Known > 0}
		if all && c.Known <= 0 {
			results[i] = Result{Tier: c.Tier, Delay: 1 << 62, OK: true}
		}
	}
	return Order(results, -1)
}

func (s *Session) Check(parent context.Context) error {
	return s.check(parent, s.opts.Retries)
}

func (s *Session) Suspect(parent context.Context) error {
	return s.check(parent, s.opts.Retries[:1])
}

func (s *Session) check(parent context.Context, waits []time.Duration) error {
	if !s.op.TryLock() {
		return nil
	}
	s.mu.Lock()
	epoch, waiting, up := s.epoch, s.waiting, s.up
	active, order, pinned, cands := s.active, s.order, s.pinned, s.cands
	s.mu.Unlock()
	if waiting > 0 || !up {
		s.op.Unlock()
		return nil
	}
	ctx, done := s.bind(parent, epoch)
	defer done()
	server := ""
	err := error(&engine.Error{Code: supervisor.CodeUnavailable, Err: errors.New("no server is running")})
	if active >= 0 {
		server = cands[active].ID
		for {
			seen := s.net.Load()
			probe, stop := context.WithCancel(ctx)
			s.mu.Lock()
			s.halt = stop
			s.mu.Unlock()
			var delay time.Duration
			delay, err = s.confirm(probe, s.limit(), waits)
			s.mu.Lock()
			s.halt = nil
			s.mu.Unlock()
			stop()
			if err == nil {
				s.set(Event{State: Connected, Server: server, Delay: delay})
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if s.net.Load() == seen || (s.opts.Online != nil && !s.opts.Online()) {
				break
			}
		}
	}
	if s.opts.Online != nil && !s.opts.Online() {
		s.set(Event{State: Offline, Server: server})
		return err
	}
	if pinned && !engineDown(err) {
		s.set(Event{State: Stalled, Server: server, Code: CodeUnreachable, Error: err.Error()})
		return &engine.Error{Code: CodeUnreachable, Err: err}
	}
	if s.opts.Bypass && len(order) < len(cands) {
		order = whole(order, len(cands))
		s.mu.Lock()
		s.order = order
		s.mu.Unlock()
	}
	from := 0
	for i, c := range order {
		if c == active {
			from = i
			if !engineDown(err) {
				from = i + 1
			}
		}
	}
	next := make([]int, len(order))
	for i := range order {
		next[i] = order[(from+i)%len(order)]
	}
	if s.opts.Bypass && active >= 0 && !engineDown(err) {
		return s.move(ctx, cands, next[:len(next)-1], server, err)
	}
	return s.walk(ctx, Switching, next)
}

const nearest = 3

func (s *Session) move(ctx context.Context, cands []Candidate, others []int, server string, cause error) error {
	for _, part := range [][]int{others[:min(nearest, len(others))], others[min(nearest, len(others)):]} {
		if len(part) == 0 {
			continue
		}
		asked := make([]Candidate, len(part))
		for i, c := range part {
			asked[i] = cands[c]
		}
		measured := s.Measure(ctx, asked)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		results := make([]Result, len(part))
		for i, m := range measured {
			results[i] = Result{Tier: asked[i].Tier, Delay: m.Delay, OK: m.OK()}
		}
		answered := Order(results, -1)
		if len(answered) == 0 {
			continue
		}
		if s.opts.OnMeasure != nil {
			s.opts.OnMeasure(asked, measured)
		}
		order := make([]int, len(answered))
		for i, a := range answered {
			order[i] = part[a]
		}
		return s.walk(ctx, Switching, order)
	}
	s.set(Event{State: Stalled, Server: server, Code: CodeNoServer, Error: cause.Error()})
	return &engine.Error{Code: CodeNoServer, Err: cause}
}

func (s *Session) Rebase(cands []Candidate) bool {
	s.op.Lock()
	defer s.op.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.up || len(s.cands) == 0 {
		return false
	}
	at := make(map[string]int, len(cands))
	for i, c := range cands {
		at[c.ID] = i
	}
	active := -1
	if s.active >= 0 {
		was := s.cands[s.active]
		i, ok := at[was.ID]
		if !ok || !bytes.Equal(cands[i].Run, was.Run) {
			return true
		}
		active = i
	}
	order := make([]int, 0, len(s.order))
	for _, i := range s.order {
		if n, ok := at[s.cands[i].ID]; ok {
			order = append(order, n)
		}
	}
	s.cands, s.order, s.active = cands, order, active
	return false
}

func whole(order []int, n int) []int {
	in := make([]bool, n)
	out := make([]int, 0, n)
	for _, i := range order {
		in[i] = true
		out = append(out, i)
	}
	for i := range n {
		if !in[i] {
			out = append(out, i)
		}
	}
	return out
}

func (s *Session) Network(online bool) {
	s.net.Add(1)
	if online {
		return
	}
	s.mu.Lock()
	if s.halt != nil {
		s.halt()
	}
	s.mu.Unlock()
	if !s.op.TryLock() {
		return
	}
	defer s.op.Unlock()
	s.mu.Lock()
	up, waiting, active, cands := s.up, s.waiting, s.active, s.cands
	s.mu.Unlock()
	if up && waiting == 0 && active >= 0 {
		s.set(Event{State: Offline, Server: cands[active].ID})
	}
}

func (s *Session) Disconnect() error {
	s.preempt()
	s.opts.Tunnel.Stop()
	s.acquire()
	defer s.op.Unlock()
	s.mu.Lock()
	s.cands, s.order, s.active, s.up, s.held, s.pinned, s.last = nil, nil, -1, false, false, false, 0
	s.mu.Unlock()
	err := s.opts.Tunnel.Stop()
	s.set(Event{State: Idle})
	return err
}

func (s *Session) preempt() context.Context {
	epoch, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.epoch, s.cancel = epoch, cancel
	s.waiting++
	s.mu.Unlock()
	return epoch
}

func (s *Session) acquire() {
	s.op.Lock()
	s.mu.Lock()
	s.waiting--
	s.mu.Unlock()
}

func (s *Session) bind(parent, epoch context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(epoch, cancel)
	return ctx, func() {
		stop()
		cancel()
		s.op.Unlock()
	}
}

func (s *Session) walk(ctx context.Context, during State, order []int) error {
	s.mu.Lock()
	pinned := s.pinned
	s.mu.Unlock()
	waits := s.opts.Retries
	if len(order) > 1 {
		waits = waits[:1]
	}
	last := errors.New("no server answered")
	for _, i := range order {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err, ok := s.settle(ctx, s.try(ctx, during, i, s.opts.Timeout, waits))
		if ok {
			return err
		}
		last = err
	}
	code := CodeNoServer
	if pinned {
		code = CodeUnreachable
	}
	return s.lost(ctx, code, last)
}

func (s *Session) try(ctx context.Context, during State, i int, limit time.Duration, waits []time.Duration) error {
	s.mu.Lock()
	c := s.cands[i]
	s.mu.Unlock()
	s.set(Event{State: during, Server: c.ID})
	if _, err := s.opts.Tunnel.Start(c.Run); err != nil {
		return err
	}
	s.mu.Lock()
	s.up, s.active, s.last = true, i, 0
	s.held = s.held || s.strict
	s.mu.Unlock()
	delay, err := s.confirm(ctx, limit, waits)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.held = true
	s.mu.Unlock()
	s.set(Event{State: Connected, Server: c.ID, Delay: delay})
	return nil
}

func (s *Session) lost(ctx context.Context, code engine.Code, last error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s.mu.Lock()
	keep := s.up && s.held
	if !keep {
		s.up, s.held, s.active = false, false, -1
	}
	s.mu.Unlock()
	if keep {
		s.set(Event{State: Stalled, Code: code, Error: last.Error()})
	} else {
		s.opts.Tunnel.Stop()
		s.set(Event{State: Failed, Code: code, Error: last.Error()})
	}
	return &engine.Error{Code: code, Err: last}
}

func (s *Session) limit() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last <= 0 {
		return s.opts.Timeout
	}
	return min(max(5*s.last, 2*time.Second), s.opts.Timeout)
}

func (s *Session) confirm(ctx context.Context, limit time.Duration, waits []time.Duration) (time.Duration, error) {
	var last error
	for _, wait := range waits {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(wait):
		}
		delay, err := s.opts.Tunnel.Probe(ctx, "", s.opts.URL, limit)
		if err == nil {
			s.mu.Lock()
			s.last = delay
			s.mu.Unlock()
			return delay, nil
		}
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		last = err
	}
	return 0, last
}

func engineDown(err error) bool {
	var e *engine.Error
	return errors.As(err, &e) && (e.Code == supervisor.CodeUnavailable || e.Code == engine.CodeIdle)
}

func (s *Session) set(e Event) {
	s.mu.Lock()
	s.state = e.State
	s.mu.Unlock()
	if s.opts.OnEvent != nil {
		s.opts.OnEvent(e)
	}
}
