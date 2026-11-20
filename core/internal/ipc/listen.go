package ipc

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/app"
	"github.com/SpofyJet/spofy-app/core/internal/host"
)

const (
	backlog    = 512
	sendBuffer = 256 << 10
	writeWait  = 10 * time.Second
)

type queue struct {
	lines chan []byte
	once  sync.Once
	drop  func()
}

func (q *queue) Write(b []byte) (int, error) {
	select {
	case q.lines <- bytes.Clone(b):
		return len(b), nil
	default:
		q.once.Do(q.drop)
		return 0, errors.New("the client does not read")
	}
}

func (q *queue) pump(c net.Conn) {
	failed := false
	for b := range q.lines {
		if failed {
			continue
		}
		c.SetWriteDeadline(time.Now().Add(writeWait))
		if _, err := c.Write(b); err != nil {
			failed = true
			q.once.Do(q.drop)
		}
	}
}

func Listen(ctx context.Context, l net.Listener, a *app.App, p *host.Platform) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var mu sync.Mutex
	open := map[net.Conn]struct{}{}
	var clients sync.WaitGroup
	go func() {
		<-ctx.Done()
		l.Close()
		mu.Lock()
		for c := range open {
			c.Close()
		}
		mu.Unlock()
	}()
	var failure error
	for {
		c, err := l.Accept()
		if err != nil {
			if ctx.Err() == nil {
				failure = err
			}
			break
		}
		mu.Lock()
		open[c] = struct{}{}
		mu.Unlock()
		clients.Add(1)
		go func() {
			defer clients.Done()
			if b, ok := c.(interface{ SetWriteBuffer(int) error }); ok {
				b.SetWriteBuffer(sendBuffer)
			}
			in, drop := inbox(c)
			out := &queue{lines: make(chan []byte, backlog), drop: func() { c.Close() }}
			pumped := make(chan struct{})
			go func() {
				out.pump(c)
				close(pumped)
			}()
			Serve(ctx, in, out, a, p)
			close(out.lines)
			<-pumped
			drop()
			c.Close()
			mu.Lock()
			delete(open, c)
			mu.Unlock()
		}()
	}
	cancel()
	clients.Wait()
	return failure
}
