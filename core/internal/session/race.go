package session

import (
	"context"
	"sync"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/supervisor"
)

type race struct {
	mu    sync.Mutex
	cands []Candidate
	began []time.Time
	done  []bool
	best  int
	delay time.Duration
	took  time.Duration
	timer *time.Timer
	over  bool
	stop  context.CancelFunc
}

func (s *Session) race(parent context.Context, cands []Candidate) []supervisor.Measurement {
	ctx, stop := context.WithCancel(parent)
	defer stop()
	r := &race{cands: cands, began: make([]time.Time, len(cands)), done: make([]bool, len(cands)), best: -1, stop: stop}
	defer r.end()
	return s.measure(ctx, cands, s.opts.Parallel, r.start, r.finish)
}

func (r *race) start(i int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.began[i] = time.Now()
	r.judge()
}

func (r *race) finish(i int, m supervisor.Measurement) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done[i] = true
	if m.OK() && (r.best < 0 || r.cands[i].Tier < r.cands[r.best].Tier || r.cands[i].Tier == r.cands[r.best].Tier && m.Delay < r.delay) {
		r.best, r.delay, r.took = i, m.Delay, m.Delay
		if !r.began[i].IsZero() {
			r.took = time.Since(r.began[i])
		}
	}
	r.judge()
}

func (r *race) judge() {
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	if r.over || r.best < 0 {
		return
	}
	var wait time.Duration
	for i, c := range r.cands {
		switch {
		case r.done[i]:
			continue
		case r.began[i].IsZero() || c.Tier < r.cands[r.best].Tier:
			return
		}
		wait = max(wait, 2*r.took+100*time.Millisecond-time.Since(r.began[i]))
	}
	if wait <= 0 {
		r.stop()
		return
	}
	r.timer = time.AfterFunc(wait, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.judge()
	})
}

func (r *race) end() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.over = true
	if r.timer != nil {
		r.timer.Stop()
	}
}
