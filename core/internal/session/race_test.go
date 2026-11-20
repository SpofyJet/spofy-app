package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/supervisor"
)

type paced struct {
	*fake
	workers int
	seen    []supervisor.Measurement
}

func (p *paced) Measure(ctx context.Context, targets []supervisor.Target, o supervisor.MeasureOptions, each func(int, supervisor.Measurement)) {
	workers := p.workers
	if workers == 0 {
		workers = len(targets)
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if o.Began != nil {
					o.Began(i)
				}
				p.mu.Lock()
				wall, alive := p.delays[string(targets[i].Config)]
				p.mu.Unlock()
				var answer <-chan time.Time
				if alive {
					answer = time.After(wall)
				}
				select {
				case <-answer:
					each(i, supervisor.Measurement{Delay: wall})
				case <-ctx.Done():
					each(i, supervisor.Measurement{Code: supervisor.CodeCanceled})
				}
			}
		}()
	}
	for i := range targets {
		select {
		case jobs <- i:
			continue
		case <-ctx.Done():
		}
		break
	}
	close(jobs)
	wg.Wait()
}

func pace(workers int, delays map[string]time.Duration) (*paced, *Session) {
	p := &paced{fake: &fake{delays: delays}, workers: workers}
	return p, New(Options{Tunnel: p, Retries: []time.Duration{0}, OnMeasure: func(_ []Candidate, m []supervisor.Measurement) { p.seen = m }})
}

func TestAnEntryThatNeverAnswersDoesNotHoldTheFirstConnection(t *testing.T) {
	p, s := pace(0, map[string]time.Duration{"de": ms(30), "nl": ms(10)})
	began := time.Now()
	if err := s.Connect(context.Background(), cands("de", "ghost", "nl", "gone"), ""); err != nil || p.now() != "nl" {
		t.Fatalf("err %v, running %q", err, p.now())
	}
	if took := time.Since(began); took > time.Second {
		t.Fatalf("the connection waited %s for entries that never answer", took)
	}
	if !p.seen[0].OK() || !p.seen[2].OK() || p.seen[1].Code != supervisor.CodeCanceled || p.seen[3].Code != supervisor.CodeCanceled {
		t.Fatalf("measured %+v", p.seen)
	}
}

func TestASlowerServerThatAnswersInTimeStaysInReserve(t *testing.T) {
	p, s := pace(0, map[string]time.Duration{"de": ms(70), "nl": ms(10)})
	p.broken = map[string]bool{"nl": true}
	if err := s.Connect(context.Background(), cands("de", "ghost", "nl"), ""); err != nil || p.now() != "de" {
		t.Fatalf("err %v, running %q", err, p.now())
	}
}

func TestAnEntryThatHasNotBeenAskedYetIsWaitedFor(t *testing.T) {
	p, s := pace(1, map[string]time.Duration{"de": ms(60), "nl": ms(10)})
	if err := s.Connect(context.Background(), cands("de", "nl"), ""); err != nil || p.now() != "nl" {
		t.Fatalf("err %v, running %q", err, p.now())
	}
}

func TestABetterTierIsWaitedForEvenWhenTheReserveAnswersFirst(t *testing.T) {
	c := cands("bypass", "de")
	c[0].Tier = 1
	p, s := pace(0, map[string]time.Duration{"bypass": ms(5), "de": ms(400)})
	if err := s.Connect(context.Background(), c, ""); err != nil || p.now() != "de" {
		t.Fatalf("err %v, running %q", err, p.now())
	}
}

func TestTheReserveTierIsAskedBeforeTheChoiceAndStaysInReserve(t *testing.T) {
	c := cands("de", "bypass", "ghost")
	c[1].Tier, c[2].Tier = 1, 1
	p, s := pace(1, map[string]time.Duration{"de": ms(10), "bypass": ms(5)})
	began := time.Now()
	if err := s.Connect(context.Background(), c, ""); err != nil || p.now() != "de" {
		t.Fatalf("err %v, running %q", err, p.now())
	}
	if took := time.Since(began); took > time.Second {
		t.Fatalf("the connection waited %s for a reserve entry that never answers", took)
	}
	p.set(func() { p.dead = down("de") })
	if err := s.Check(context.Background()); err != nil || p.now() != "bypass" {
		t.Fatalf("err %v, running %q", err, p.now())
	}
}

func TestDelaysAreReportedAsTheyArrive(t *testing.T) {
	_, s := pace(0, map[string]time.Duration{"de": ms(400), "nl": ms(10)})
	var mu sync.Mutex
	var early [][]supervisor.Measurement
	out := s.MeasureLive(context.Background(), cands("de", "nl"), 20*time.Millisecond, func(part []supervisor.Measurement) {
		mu.Lock()
		early = append(early, part)
		mu.Unlock()
	})
	if !out[0].OK() || !out[1].OK() {
		t.Fatalf("measured %+v", out)
	}
	if len(early) == 0 || early[0][0].Code != supervisor.CodeCanceled || !early[0][1].OK() {
		t.Fatalf("reported before the end: %+v", early)
	}
}
