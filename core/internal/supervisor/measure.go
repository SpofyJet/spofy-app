package supervisor

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/engine"
)

const CodeCanceled engine.Code = "canceled"

type Measurement struct {
	Delay time.Duration
	Code  engine.Code
	Error string
}

func (m Measurement) OK() bool { return m.Code == "" }

type Target struct {
	Config []byte
	Tag    string
}

type MeasureOptions struct {
	URL      string
	Timeout  time.Duration
	Parallel int
}

func (s *Supervisor) Probe(ctx context.Context, tag, url string, timeout time.Duration) (time.Duration, error) {
	p := s.current()
	if p == nil || p.dead() {
		return 0, &engine.Error{Code: CodeUnavailable, Err: errors.New("engine is not running")}
	}
	r, err := p.callContext(ctx, engine.Request{Op: "probe", Tag: tag, URL: url, Timeout: timeout.Milliseconds()}, timeout+2*time.Second)
	if ctx.Err() != nil {
		return 0, &engine.Error{Code: CodeCanceled, Err: ctx.Err()}
	}
	if err != nil {
		return 0, &engine.Error{Code: CodeUnavailable, Err: err}
	}
	if !r.OK {
		return 0, &engine.Error{Code: r.Code, Err: errors.New(r.Error)}
	}
	return time.Duration(r.Ms) * time.Millisecond, nil
}

func (s *Supervisor) Measure(ctx context.Context, targets []Target, o MeasureOptions, each func(int, Measurement)) {
	if o.Timeout <= 0 {
		o.Timeout = 5 * time.Second
	}
	workers := min(max(o.Parallel, 1), len(targets))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var p *process
			defer func() {
				if p != nil {
					p.kill()
					<-p.done
				}
			}()
			for i := range jobs {
				var m Measurement
				p, m = s.measure(ctx, p, targets[i], o)
				each(i, m)
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

func (s *Supervisor) measure(ctx context.Context, p *process, t Target, o MeasureOptions) (*process, Measurement) {
	req := engine.Request{Op: "measure", Config: t.Config, Tag: t.Tag, URL: o.URL, Timeout: o.Timeout.Milliseconds()}
	last := "prober exited"
	for attempt := 0; attempt < 2; attempt++ {
		if ctx.Err() != nil {
			return p, Measurement{Code: CodeCanceled, Error: ctx.Err().Error()}
		}
		if p == nil || p.dead() {
			fresh, err := s.spawn()
			if err != nil {
				return nil, Measurement{Code: CodeUnavailable, Error: err.Error()}
			}
			p = fresh
		}
		r, err := p.callContext(ctx, req, o.Timeout+5*time.Second)
		if err == nil {
			if r.OK {
				return p, Measurement{Delay: time.Duration(r.Ms) * time.Millisecond}
			}
			return p, Measurement{Code: r.Code, Error: r.Error}
		}
		last = err.Error()
		p.kill()
		<-p.done
	}
	if ctx.Err() != nil {
		return p, Measurement{Code: CodeCanceled, Error: ctx.Err().Error()}
	}
	return p, Measurement{Code: CodeUnavailable, Error: last}
}
