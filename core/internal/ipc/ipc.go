package ipc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"

	"github.com/SpofyJet/spofy-app/core/internal/app"
	"github.com/SpofyJet/spofy-app/core/internal/engine"
	"github.com/SpofyJet/spofy-app/core/internal/host"
)

const (
	CodeRequest engine.Code = "bad_request"
	CodeOp      engine.Code = "unknown_op"
)

const maxLine = 1 << 20

type Request struct {
	ID     uint64          `json:"id"`
	Op     string          `json:"op"`
	URL    string          `json:"url,omitempty"`
	Server string          `json:"server,omitempty"`
	Key    string          `json:"key,omitempty"`
	Value  json.RawMessage `json:"value,omitempty"`
	Online *bool           `json:"online,omitempty"`
	Answer uint64          `json:"answer,omitempty"`
	OK     bool            `json:"ok,omitempty"`
	Error  string          `json:"error,omitempty"`
	File   bool            `json:"file,omitempty"`
	In     bool            `json:"in,omitempty"`
	Owners bool            `json:"owners,omitempty"`
}

type Reply struct {
	Re       uint64          `json:"re"`
	OK       bool            `json:"ok"`
	Code     engine.Code     `json:"code,omitempty"`
	Error    string          `json:"error,omitempty"`
	Snapshot *app.Snapshot   `json:"snapshot,omitempty"`
	Changed  *bool           `json:"changed,omitempty"`
	Traffic  *engine.Traffic `json:"traffic,omitempty"`
	Report   *app.Report     `json:"report,omitempty"`
}

func Serve(ctx context.Context, r io.Reader, w io.Writer, a *app.App, p *host.Platform) error {
	ctx, cancel := context.WithCancel(ctx)
	var out sync.Mutex
	var jobs sync.WaitGroup
	put := func(v any) {
		if b, err := json.Marshal(v); err == nil {
			w.Write(append(b, '\n'))
		}
	}
	send := func(v any) {
		out.Lock()
		put(v)
		out.Unlock()
	}
	stop := a.Listen(func(e app.Event) { send(e) })
	detach := func() {}
	defer func() {
		detach()
		cancel()
		jobs.Wait()
		stop()
	}()
	files, _ := r.(interface{ TakeFile(string) *os.File })
	attached := false
	answer := func(id uint64, err error) {
		var e *engine.Error
		switch {
		case err == nil:
			send(Reply{Re: id, OK: true})
		case errors.As(err, &e):
			send(Reply{Re: id, Code: e.Code, Error: e.Err.Error()})
		default:
			send(Reply{Re: id, Code: CodeRequest, Error: err.Error()})
		}
	}
	later := func(id uint64, work func() error) {
		jobs.Add(1)
		go func() {
			defer jobs.Done()
			answer(id, work())
		}()
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var req Request
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		err := dec.Decode(&req)
		if err == nil && req.Answer != 0 {
			var file *os.File
			if req.File && files != nil {
				file = files.TakeFile("tun")
			}
			if attached {
				p.Answer(req.Answer, host.Answer{OK: req.OK, Error: req.Error, File: file, In: req.In})
			} else if file != nil {
				file.Close()
			}
			continue
		}
		if err != nil || req.ID == 0 || req.Op == "" {
			send(Reply{Re: req.ID, Code: CodeRequest, Error: "bad request"})
			continue
		}
		switch req.Op {
		case "platform":
			if p == nil {
				send(Reply{Re: req.ID, Code: CodeOp, Error: "unknown operation"})
				continue
			}
			if !attached {
				attached = true
				detach = p.Attach(func(ask host.Ask) { send(ask) }, req.Owners)
			}
			send(Reply{Re: req.ID, OK: true})
		case "hello":
			out.Lock()
			snap := a.Snapshot()
			put(Reply{Re: req.ID, OK: true, Snapshot: &snap})
			out.Unlock()
		case "subscribe":
			later(req.ID, func() error { return a.Subscribe(ctx, req.URL) })
		case "refresh":
			jobs.Add(1)
			go func() {
				defer jobs.Done()
				changed, err := a.Refresh(ctx)
				if err != nil {
					answer(req.ID, err)
					return
				}
				send(Reply{Re: req.ID, OK: true, Changed: &changed})
			}()
		case "unsubscribe":
			answer(req.ID, a.Unsubscribe())
		case "connect":
			later(req.ID, func() error { return a.Connect(req.Server) })
		case "disconnect":
			answer(req.ID, a.Disconnect())
		case "check":
			later(req.ID, a.Check)
		case "online":
			if req.Online == nil {
				send(Reply{Re: req.ID, Code: CodeRequest, Error: "bad request"})
				continue
			}
			later(req.ID, func() error { return a.SetOnline(*req.Online) })
		case "measure":
			later(req.ID, func() error { return a.Measure(ctx) })
		case "set":
			answer(req.ID, a.Set(req.Key, req.Value))
		case "traffic":
			t := a.Traffic()
			send(Reply{Re: req.ID, OK: true, Traffic: &t})
		case "report":
			r := a.Report()
			send(Reply{Re: req.ID, OK: true, Report: &r})
		default:
			send(Reply{Re: req.ID, Code: CodeOp, Error: "unknown operation"})
		}
	}
	return sc.Err()
}
