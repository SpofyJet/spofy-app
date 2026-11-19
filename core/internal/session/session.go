package session

import (
	"context"
	"errors"
	"sync"
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
}

type Event struct {
	State  State         `json:"state"`
	Server string        `json:"server,omitempty"`
	Delay  time.Duration `json:"delay,omitempty"`
	Code   engine.Code   `json:"code,omitempty"`
	Error  string        `json:"error,omitempty"`
}

type Options struct {
	Tunnel   Tunnel
	OnEvent  func(Event)
	Online   func() bool
	Strict   bool
	URL      string
	Timeout  time.Duration
	Retries  []time.Duration
	Parallel int
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
	pinned  bool
	state   State
	last    time.Duration
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
	return &Session{opts: o, active: -1, state: Idle}
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
	ctx, done := s.bind(parent, epoch)
	defer done()
	if pinned == "" {
		s.set(Event{State: Measuring})
		measured := s.Measure(ctx, cands)
		results := make([]Result, len(cands))
		for i, m := range measured {
			results[i] = Result{Tier: cands[i].Tier, Delay: m.Delay, OK: m.OK()}
		}
		order = Order(results, -1)
		if len(order) == 0 {
			return s.lost(ctx, Connecting, CodeNoServer, errors.New("no server answered"))
		}
	}
	s.mu.Lock()
	s.cands, s.order, s.pinned, s.active = cands, order, pinned != "", -1
	s.mu.Unlock()
	return s.walk(ctx, Connecting, order)
}

func (s *Session) Check(parent context.Context) error {
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
		var delay time.Duration
		if delay, err = s.confirm(ctx, s.limit(), s.opts.Retries); err == nil {
			s.set(Event{State: Connected, Server: server, Delay: delay})
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
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
	return s.walk(ctx, Switching, next)
}

func (s *Session) Disconnect() error {
	s.preempt()
	s.opts.Tunnel.Stop()
	s.acquire()
	defer s.op.Unlock()
	s.mu.Lock()
	s.cands, s.order, s.active, s.up, s.pinned, s.last = nil, nil, -1, false, false, 0
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
	cands, pinned := s.cands, s.pinned
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
		c := cands[i]
		s.set(Event{State: during, Server: c.ID})
		if _, err := s.opts.Tunnel.Start(c.Run); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, supervisor.ErrSuperseded) {
				return err
			}
			last = err
			continue
		}
		s.mu.Lock()
		s.up, s.active, s.last = true, i, 0
		s.mu.Unlock()
		delay, err := s.confirm(ctx, s.opts.Timeout, waits)
		if err == nil {
			s.set(Event{State: Connected, Server: c.ID, Delay: delay})
			return nil
		}
		last = err
	}
	code := CodeNoServer
	if pinned {
		code = CodeUnreachable
	}
	return s.lost(ctx, during, code, last)
}

func (s *Session) lost(ctx context.Context, during State, code engine.Code, last error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s.mu.Lock()
	keep := s.up && (s.opts.Strict || during == Switching)
	if !keep {
		s.up, s.active = false, -1
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
