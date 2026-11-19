package engine

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"
)

type Request struct {
	ID     uint64          `json:"id"`
	Op     string          `json:"op"`
	Config json.RawMessage `json:"config,omitempty"`
	Tag    string          `json:"tag,omitempty"`
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

const maxLine = 4 << 20

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
	stop := func() {
		if inst != nil {
			inst.Close()
			inst = nil
		}
	}
	defer stop()

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
			stop()
			next, err := Start(req.Config)
			if err != nil {
				send(failure(req.ID, err))
				continue
			}
			inst = next
			send(Reply{Re: req.ID, OK: true, Ms: time.Since(began).Milliseconds()})
		case "stop":
			stop()
			send(Reply{Re: req.ID, OK: true})
		case "traffic":
			t := Traffic{}
			if inst != nil {
				t = inst.Traffic(req.Tag)
			}
			send(Reply{Re: req.ID, OK: true, Traffic: &t})
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
