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
	sends int
	until time.Time
	rest  time.Duration
}

func (w *stall) step(t engine.Traffic, quiet time.Duration, now time.Time) bool {
	sent, got := t.Up > w.seen.Up, t.Down > w.seen.Down
	back := t.Up < w.seen.Up || t.Down < w.seen.Down
	w.seen = t
	if got || back {
		w.since, w.sends, w.rest = time.Time{}, 0, 0
		return false
	}
	if sent {
		if w.since.IsZero() {
			w.since = now
		}
		w.sends++
	}
	if w.sends < 2 || now.Sub(w.since) < quiet || now.Before(w.until) {
		return false
	}
	w.rest = min(max(2*w.rest, stallRest), stallRestMax)
	w.until = now.Add(w.rest)
	w.since, w.sends = time.Time{}, 0
	return true
}
