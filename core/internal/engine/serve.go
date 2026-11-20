package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"
)

type Request struct {
	ID      uint64          `json:"id"`
	Op      string          `json:"op"`
	Config  json.RawMessage `json:"config,omitempty"`
	Tag     string          `json:"tag,omitempty"`
	URL     string          `json:"url,omitempty"`
	Timeout int64           `json:"timeout,omitempty"`
	Tun     bool            `json:"tun,omitempty"`
}

type Reply struct {
	Re      uint64   `json:"re,omitempty"`
	Ev      string   `json:"ev,omitempty"`
	OK      bool     `json:"ok,omitempty"`
	Code    Code     `json:"code,omitempty"`
	Error   string   `json:"error,omitempty"`
	Ms      int64    `json:"ms,omitempty"`
	Traffic *Traffic `json:"traffic,omitempty"`
	Line    string   `json:"line,omitempty"`
}

const (
	maxLine   = 4 << 20
	probeWait = 5 * time.Second
)

func Serve(r io.Reader, w io.Writer) error {
	var mu sync.Mutex
	enc := json.NewEncoder(w)
	send := func(m Reply) {
		mu.Lock()
		enc.Encode(m)
		mu.Unlock()
	}
	SetLogSink(func(line string) { send(Reply{Ev: "log", Line: line}) })
	defer SetLogSink(nil)

	var inst *Instance
	var probes sync.WaitGroup
	live, cancel := context.WithCancel(context.Background())
	stop := func() {
		cancel()
		probes.Wait()
		live, cancel = context.WithCancel(context.Background())
		if inst != nil {
			inst.Close()
			inst = nil
		}
	}
	defer func() {
		stop()
		cancel()
	}()
	timed := func(parent context.Context, ms int64) (context.Context, context.CancelFunc) {
		wait := probeWait
		if ms > 0 {
			wait = time.Duration(ms) * time.Millisecond
		}
		return context.WithTimeout(parent, wait)
	}
	answer := func(id uint64, took time.Duration, err error) {
		if err != nil {
			send(failure(id, err))
			return
		}
		send(Reply{Re: id, OK: true, Ms: max(took.Milliseconds(), 1)})
	}

	send(Reply{Ev: "ready"})
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	for sc.Scan() {
		var req Request
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			send(Reply{Ev: "error", Code: CodeConfig, Error: "bad request"})
			continue
		}
		switch req.Op {
		case "start":
			began := time.Now()
			release := keep()
			stop()
			tun := -1
			if req.Tun {
				if in, ok := r.(interface{ Take() int }); ok {
					tun = in.Take()
				}
				if tun < 0 {
					release()
					send(Reply{Re: req.ID, Code: CodeStart, Error: "the tunnel did not come with the request"})
					continue
				}
			}
			next, err := StartOn(req.Config, tun)
			release()
			if err != nil {
				send(failure(req.ID, err))
				continue
			}
			inst = next
			send(Reply{Re: req.ID, OK: true, Ms: max(time.Since(began).Milliseconds(), 1)})
		case "stop":
			stop()
			send(Reply{Re: req.ID, OK: true})
		case "traffic":
			t := Traffic{}
			if inst != nil {
				t = inst.Traffic(req.Tag)
			}
			send(Reply{Re: req.ID, OK: true, Traffic: &t})
		case "probe":
			if inst == nil {
				send(Reply{Re: req.ID, Code: CodeIdle, Error: "not started"})
				continue
			}
			probes.Add(1)
			go func(i *Instance, parent context.Context, req Request) {
				defer probes.Done()
				ctx, done := timed(parent, req.Timeout)
				defer done()
				took, err := i.Probe(ctx, req.URL, req.Tag)
				answer(req.ID, took, err)
			}(inst, live, req)
		case "measure":
			ctx, done := timed(context.Background(), req.Timeout)
			took, err := Measure(ctx, req.Config, req.URL, req.Tag)
			done()
			answer(req.ID, took, err)
		case "ping":
			send(Reply{Re: req.ID, OK: true})
		default:
			send(Reply{Re: req.ID, Code: CodeConfig, Error: "unknown op"})
		}
	}
	return sc.Err()
}

func failure(id uint64, err error) Reply {
	var e *Error
	if errors.As(err, &e) {
		return Reply{Re: id, Code: e.Code, Error: e.Err.Error()}
	}
	return Reply{Re: id, Code: CodeStart, Error: err.Error()}
}
