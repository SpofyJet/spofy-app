package app

import (
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/engine"
)

const (
	stallRest    = 30 * time.Second
	stallRestMax = 5 * time.Minute
)

type stall struct {
	seen  engine.Traffic
	since time.Time
	until time.Time
	rest  time.Duration
}

func (w *stall) step(t engine.Traffic, quiet time.Duration, now time.Time) bool {
	sent, got := t.Up > w.seen.Up, t.Down > w.seen.Down
	back := t.Up < w.seen.Up || t.Down < w.seen.Down
	w.seen = t
	if got || back {
		w.since, w.rest = time.Time{}, 0
		return false
	}
	if w.since.IsZero() {
		if sent {
			w.since = now
		}
		return false
	}
	if now.Sub(w.since) < quiet || now.Before(w.until) {
		return false
	}
	w.rest = min(max(2*w.rest, stallRest), stallRestMax)
	w.until = now.Add(w.rest)
	w.since = time.Time{}
	return true
}
