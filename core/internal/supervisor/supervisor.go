package supervisor

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/engine"
)

const (
	KindLog      = "log"
	KindCrashed  = "crashed"
	KindRestored = "restored"
	KindFailed   = "failed"
)

const (
	CodeCrashLoop   engine.Code = "crash_loop"
	CodeUnavailable engine.Code = "engine_unavailable"
	CodeGuard       engine.Code = "guard_failed"
)

type Guard interface {
	Raise() error
	Lower() error
}

var ErrSuperseded = errors.New("supervisor: superseded by a newer request")

type Event struct {
	Kind     string        `json:"kind"`
	Line     string        `json:"line,omitempty"`
	Stderr   string        `json:"stderr,omitempty"`
	Downtime time.Duration `json:"downtime,omitempty"`
	Code     engine.Code   `json:"code,omitempty"`
	Error    string        `json:"error,omitempty"`
}

type Options struct {
	Command     func() *exec.Cmd
	Guard       Guard
	HoldGuard   bool
	OnEvent     func(Event)
	ReadyWait   time.Duration
	StartWait   time.Duration
	CrashWindow time.Duration
	CrashLimit  int
	Backoff     []time.Duration
}

type Supervisor struct {
	opts Options
	op   sync.Mutex
	busy atomic.Bool

	mu         sync.Mutex
	gen        uint64
	want       []byte
	proc       *process
	recovering bool
	crashes    []time.Time
}

func New(o Options) *Supervisor {
	if o.ReadyWait == 0 {
		o.ReadyWait = 10 * time.Second
	}
	if o.StartWait == 0 {
		o.StartWait = 30 * time.Second
	}
	if o.CrashWindow == 0 {
		o.CrashWindow = time.Minute
	}
	if o.CrashLimit == 0 {
		o.CrashLimit = 5
	}
	if o.Backoff == nil {
		o.Backoff = []time.Duration{0, 100 * time.Millisecond, 300 * time.Millisecond, time.Second, 3 * time.Second}
	}
	return &Supervisor{opts: o}
}

func (s *Supervisor) Warm() error {
	s.op.Lock()
	defer s.op.Unlock()
	_, err := s.alive()
	return err
}

func (s *Supervisor) Start(cfg []byte) (time.Duration, error) {
	gen := s.intent(cfg)
	if s.busy.Load() {
		s.killCurrent()
	}
	s.op.Lock()
	defer s.op.Unlock()
	if s.generation() != gen {
		return 0, ErrSuperseded
	}
	if err := s.raise(); err != nil {
		s.clear(gen)
		return 0, err
	}
	took, err := s.bringUp(cfg)
	if err != nil && s.generation() == gen {
		s.clear(gen)
		s.lower()
	}
	return took, err
}

func (s *Supervisor) raise() error {
	if s.opts.Guard == nil {
		return nil
	}
	if err := s.opts.Guard.Raise(); err != nil {
		s.opts.Guard.Lower()
		return &engine.Error{Code: CodeGuard, Err: err}
	}
	return nil
}

func (s *Supervisor) lower() {
	if s.opts.Guard != nil {
		s.opts.Guard.Lower()
	}
}

func (s *Supervisor) Stop() error {
	s.intent(nil)
	if s.busy.Load() {
		s.killCurrent()
	}
	s.op.Lock()
	defer s.op.Unlock()
	defer s.lower()
	p := s.current()
	if p == nil || p.dead() {
		return nil
	}
	p.armed.Store(false)
	if _, err := p.call(engine.Request{Op: "stop"}, 5*time.Second); err != nil {
		p.kill()
	}
	return nil
}

func (s *Supervisor) Close() error {
	s.intent(nil)
	s.killIfBusy()
	s.op.Lock()
	defer s.op.Unlock()
	defer s.lower()
	p := s.current()
	if p == nil {
		return nil
	}
	p.armed.Store(false)
	p.killed.Store(true)
	p.in.Close()
	select {
	case <-p.done:
	case <-time.After(3 * time.Second):
		p.kill()
		<-p.done
	}
	s.mu.Lock()
	s.proc = nil
	s.mu.Unlock()
	return nil
}

func (s *Supervisor) Traffic(tag string) (engine.Traffic, error) {
	p := s.current()
	if p == nil || p.dead() {
		return engine.Traffic{}, nil
	}
	r, err := p.call(engine.Request{Op: "traffic", Tag: tag}, 2*time.Second)
	if err != nil || r.Traffic == nil {
		return engine.Traffic{}, err
	}
	return *r.Traffic, nil
}

func (s *Supervisor) Pid() int {
	p := s.current()
	if p == nil || p.dead() {
		return 0
	}
	return p.cmd.Process.Pid
}

func (s *Supervisor) killIfBusy() {
	if s.busy.Load() {
		s.killCurrent()
	}
}

func (s *Supervisor) killCurrent() {
	if p := s.current(); p != nil {
		p.kill()
	}
}

func (s *Supervisor) intent(cfg []byte) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gen++
	s.want = cfg
	return s.gen
}

func (s *Supervisor) clear(gen uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen == gen {
		s.want = nil
	}
}

func (s *Supervisor) generation() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen
}

func (s *Supervisor) current() *process {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.proc
}

func (s *Supervisor) emit(e Event) {
	if s.opts.OnEvent != nil {
		s.opts.OnEvent(e)
	}
}

func (s *Supervisor) alive() (*process, error) {
	p := s.current()
	if p != nil && !p.dead() {
		return p, nil
	}
	p, err := s.spawn()
	if err != nil {
		return nil, &engine.Error{Code: CodeUnavailable, Err: err}
	}
	s.mu.Lock()
	s.proc = p
	s.mu.Unlock()
	return p, nil
}

func (s *Supervisor) bringUp(cfg []byte) (time.Duration, error) {
	s.busy.Store(true)
	defer s.busy.Store(false)
	began := time.Now()
	p, err := s.alive()
	if err != nil {
		return 0, err
	}
	p.armed.Store(false)
	r, err := p.call(engine.Request{Op: "start", Config: cfg}, s.opts.StartWait)
	if err != nil {
		p.kill()
		return 0, &engine.Error{Code: CodeUnavailable, Err: err}
	}
	if !r.OK {
		return 0, &engine.Error{Code: r.Code, Err: errors.New(r.Error)}
	}
	p.armed.Store(true)
	return time.Since(began), nil
}

func (s *Supervisor) exited(p *process) {
	s.mu.Lock()
	cur := s.proc == p
	cfg, gen := s.want, s.gen
	if !cur || cfg == nil || !p.armed.Load() || p.killed.Load() || s.recovering {
		s.mu.Unlock()
		return
	}
	now := time.Now()
	fresh := s.crashes[:0]
	for _, t := range s.crashes {
		if now.Sub(t) < s.opts.CrashWindow {
			fresh = append(fresh, t)
		}
	}
	s.crashes = append(fresh, now)
	loop := len(s.crashes) >= s.opts.CrashLimit
	if loop {
		s.want = nil
		s.crashes = nil
	} else {
		s.recovering = true
	}
	s.mu.Unlock()
	s.emit(Event{Kind: KindCrashed, Stderr: p.stderr.String()})
	if loop {
		s.op.Lock()
		if s.generation() == gen && !s.opts.HoldGuard {
			s.lower()
		}
		s.op.Unlock()
		s.emit(Event{Kind: KindFailed, Code: CodeCrashLoop, Error: "engine keeps crashing"})
		return
	}
	go s.recover(gen, cfg, now)
}

func (s *Supervisor) recover(gen uint64, cfg []byte, at time.Time) {
	defer func() {
		s.mu.Lock()
		s.recovering = false
		s.mu.Unlock()
	}()
	for attempt := 0; ; attempt++ {
		time.Sleep(s.opts.Backoff[min(attempt, len(s.opts.Backoff)-1)])
		s.op.Lock()
		if s.generation() != gen {
			s.op.Unlock()
			return
		}
		_, err := s.bringUp(cfg)
		var e *engine.Error
		fatal := err != nil && errors.As(err, &e) && e.Code != CodeUnavailable
		giveUp := fatal || (err != nil && attempt+1 >= s.opts.CrashLimit)
		if giveUp {
			s.clear(gen)
			if !s.opts.HoldGuard {
				s.lower()
			}
		}
		s.op.Unlock()
		switch {
		case err == nil:
			s.emit(Event{Kind: KindRestored, Downtime: time.Since(at)})
			return
		case fatal:
			s.emit(Event{Kind: KindFailed, Code: e.Code, Error: e.Err.Error()})
			return
		case giveUp:
			s.emit(Event{Kind: KindFailed, Code: CodeCrashLoop, Error: err.Error()})
			return
		}
	}
}

type process struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	stderr *tail
	ready  chan struct{}
	done   chan struct{}
	armed  atomic.Bool
	killed atomic.Bool

	mu      sync.Mutex
	next    uint64
	pending map[uint64]chan engine.Reply
}

func (s *Supervisor) spawn() (*process, error) {
	cmd := s.opts.Command()
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	p := &process{
		cmd:     cmd,
		in:      in,
		stderr:  &tail{max: 32 << 10},
		ready:   make(chan struct{}),
		done:    make(chan struct{}),
		pending: map[uint64]chan engine.Reply{},
	}
	cmd.Stderr = p.stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	drained := make(chan struct{})
	go func() {
		p.read(out, s.emit)
		close(drained)
	}()
	go func() {
		<-drained
		cmd.Wait()
		close(p.done)
		p.fail()
		s.exited(p)
	}()
	select {
	case <-p.ready:
		return p, nil
	case <-p.done:
		return nil, errors.New("engine exited during startup: " + p.stderr.String())
	case <-time.After(s.opts.ReadyWait):
		p.kill()
		return nil, errors.New("engine did not become ready")
	}
}

func (p *process) read(r io.Reader, emit func(Event)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	once := sync.Once{}
	for sc.Scan() {
		var m engine.Reply
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		switch {
		case m.Ev == "ready":
			once.Do(func() { close(p.ready) })
		case m.Ev == "log":
			emit(Event{Kind: KindLog, Line: m.Line})
		case m.Re != 0:
			p.mu.Lock()
			ch := p.pending[m.Re]
			delete(p.pending, m.Re)
			p.mu.Unlock()
			if ch != nil {
				ch <- m
			}
		}
	}
}

func (p *process) call(req engine.Request, wait time.Duration) (engine.Reply, error) {
	ch := make(chan engine.Reply, 1)
	p.mu.Lock()
	p.next++
	req.ID = p.next
	p.pending[req.ID] = ch
	line, err := json.Marshal(req)
	if err == nil {
		_, err = p.in.Write(append(line, '\n'))
	}
	p.mu.Unlock()
	if err != nil {
		p.drop(req.ID)
		return engine.Reply{}, err
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case r := <-ch:
		return r, nil
	case <-p.done:
		return engine.Reply{}, errors.New("engine exited: " + p.stderr.String())
	case <-timer.C:
		p.drop(req.ID)
		return engine.Reply{}, errors.New("engine did not answer in time")
	}
}

func (p *process) drop(id uint64) {
	p.mu.Lock()
	delete(p.pending, id)
	p.mu.Unlock()
}

func (p *process) fail() {
	p.mu.Lock()
	p.pending = map[uint64]chan engine.Reply{}
	p.mu.Unlock()
}

func (p *process) dead() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func (p *process) kill() {
	p.killed.Store(true)
	p.cmd.Process.Kill()
}

type tail struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (t *tail) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if extra := len(t.buf) - t.max; extra > 0 {
		t.buf = append(t.buf[:0], t.buf[extra:]...)
	}
	return len(b), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
