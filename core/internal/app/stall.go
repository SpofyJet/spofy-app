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
	fired time.Time
	rest  time.Duration
}

func (w *stall) step(t engine.Traffic, quiet time.Duration, now time.Time) bool {
	sent, got := t.Up > w.seen.Up, t.Back > w.seen.Back
	back := t.Up < w.seen.Up || t.Back < w.seen.Back
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
	if w.sends < 2 || now.Sub(w.since) < quiet || (!w.fired.IsZero() && now.Sub(w.fired) < max(w.rest, stallRest)) {
		return false
	}
	w.rest = min(max(2*w.rest, stallRest), stallRestMax)
	w.fired = now
	w.since, w.sends = time.Time{}, 0
	return true
}
