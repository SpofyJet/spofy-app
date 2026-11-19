package app

import (
	"testing"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/engine"
)

func TestStallAsksForACheckOnlyWhenTrafficGoesOutAndNothingComesBack(t *testing.T) {
	at := time.Unix(1000, 0)
	quiet := 6 * time.Second
	var w stall
	var up, down int64
	step := func(after time.Duration, sent, got int64) bool {
		at = at.Add(after)
		up, down = up+sent, down+got
		return w.step(engine.Traffic{Up: up, Down: down}, quiet, at)
	}
	fired := func(want bool, got bool, what string) {
		t.Helper()
		if got != want {
			t.Fatalf("%s: fired=%v, want %v", what, got, want)
		}
	}
	for i := 0; i < 30; i++ {
		fired(false, step(2*time.Second, 500, 9000), "traffic in both directions")
	}
	for i := 0; i < 30; i++ {
		fired(false, step(2*time.Second, 0, 0), "no traffic at all")
	}
	fired(false, step(2*time.Second, 300, 0), "one request that waits for its answer")
	for i := 0; i < 30; i++ {
		fired(false, step(2*time.Second, 0, 0), "a single send is not a sign, however long the wait")
	}
	fired(true, step(2*time.Second, 300, 0), "a second send, still nothing back")
	fired(false, step(40*time.Second, 300, 4000), "an answer came")
	fired(false, step(2*time.Second, 300, 0), "the first unanswered send")
	fired(false, step(2*time.Second, 300, 0), "the second one, 2 s of silence")
	fired(false, step(2*time.Second, 0, 0), "4 s of silence")
	fired(true, step(2*time.Second, 0, 0), "6 s of silence after two sends")
	for i := 0; i < 14; i++ {
		fired(false, step(2*time.Second, 300, 0), "the pause after a check")
	}
	fired(true, step(2*time.Second, 300, 0), "30 s later, still nothing back")
	for i := 0; i < 29; i++ {
		fired(false, step(2*time.Second, 300, 0), "the pause doubles")
	}
	fired(true, step(2*time.Second, 300, 0), "60 s later")
	fired(false, step(2*time.Second, 300, 4000), "an answer came")
	fired(false, step(2*time.Second, 300, 0), "a new unanswered send")
	fired(false, step(4*time.Second, 0, 100), "answered within the limit")
	up, down = 0, 0
	fired(false, step(2*time.Second, 50, 0), "counters started again with a new engine")
	fired(false, step(6*time.Second, 0, 0), "the send right after the restart was the baseline")

	var long stall
	at, up, down = time.Unix(5000, 0), 0, 0
	w = long
	checks := 0
	for i := 0; i < 30*60/2; i++ {
		if step(2*time.Second, 1000, 0) {
			checks++
		}
	}
	if checks < 6 || checks > 10 {
		t.Fatalf("half an hour of one-way traffic asked for %d checks, want 6 to 10", checks)
	}
}
