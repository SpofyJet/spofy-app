//go:build unix || windows

package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/host"
)

type client struct {
	t    *testing.T
	conn net.Conn
	sc   *bufio.Scanner
}

func dial(t *testing.T, path string) *client {
	t.Helper()
	c, err := dialChannel(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	return &client{t: t, conn: c, sc: sc}
}

func (c *client) send(line string) {
	c.t.Helper()
	if _, err := c.conn.Write([]byte(line + "\n")); err != nil {
		c.t.Fatal(err)
	}
}

func (c *client) until(want func(map[string]any) bool) map[string]any {
	c.t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for c.sc.Scan() {
		var m map[string]any
		if json.Unmarshal(c.sc.Bytes(), &m) == nil && want(m) {
			return m
		}
	}
	c.t.Fatalf("the channel ended before the expected line: %v", c.sc.Err())
	return nil
}

func (c *client) reply(id int) map[string]any {
	c.t.Helper()
	return c.until(func(m map[string]any) bool { return m["re"] == float64(id) })
}

func listen(t *testing.T, r *rig, p *host.Platform) (string, context.CancelFunc, chan error) {
	t.Helper()
	l, path := listenChannel(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Listen(ctx, l, r.app, p) }()
	t.Cleanup(func() {
		cancel()
		<-done
		done <- nil
	})
	return path, cancel, done
}

func TestChannelServesEveryClientAndStopsWithThem(t *testing.T) {
	r := newRig(t)
	go func() {
		for range r.lines {
		}
	}()
	path, cancel, done := listen(t, r, nil)
	ui, other := dial(t, path), dial(t, path)
	ui.send(`{"id":1,"op":"hello"}`)
	if m := ui.reply(1); m["ok"] != true || m["snapshot"] == nil {
		t.Fatalf("hello: %v", m)
	}
	other.send(`{"id":7,"op":"set","key":"theme","value":"dark"}`)
	if m := other.reply(7); m["ok"] != true {
		t.Fatalf("set: %v", m)
	}
	ui.until(func(m map[string]any) bool { return m["ev"] == "settings" })
	other.conn.Close()
	ui.send(`{"id":2,"op":"hello"}`)
	if m := ui.reply(2); m["ok"] != true {
		t.Fatalf("hello after another client left: %v", m)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the listener did not stop")
	}
	ui.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if ui.sc.Scan() {
		t.Fatalf("a line after the listener stopped: %s", ui.sc.Text())
	}
	done <- nil
}

func TestClientThatDoesNotReadIsDropped(t *testing.T) {
	r := newRig(t)
	go func() {
		for range r.lines {
		}
	}()
	path, _, _ := listen(t, r, nil)
	frozen, live := dial(t, path), dial(t, path)
	for _, c := range []*client{frozen, live} {
		c.send(`{"id":1,"op":"hello"}`)
	}
	live.reply(1)
	time.Sleep(50 * time.Millisecond)
	value := `"` + strings.Repeat("x", 4000) + `"`
	var read atomic.Int64
	seen := make(chan int, 1)
	go func() {
		live.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		for live.sc.Scan() {
			if strings.Contains(live.sc.Text(), `"ev":"settings"`) && read.Add(1) == 1000 {
				break
			}
		}
		seen <- int(read.Load())
	}()
	var busy time.Duration
	for count := 1; count <= 1000; count++ {
		began := time.Now()
		if err := r.app.Set("note", json.RawMessage(value)); err != nil {
			t.Fatal(err)
		}
		busy += time.Since(began)
		if count%(backlog/2) == 0 {
			for wait := time.Now().Add(20 * time.Second); read.Load() < int64(count) && time.Now().Before(wait); {
				time.Sleep(time.Millisecond)
			}
		}
	}
	if busy > 20*time.Second {
		t.Fatalf("a client that does not read slowed the core down: %v", busy)
	}
	if n := <-seen; n != 1000 {
		t.Fatalf("the reading client got %d of 1000 events", n)
	}
	frozen.conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	lines := 0
	for frozen.sc.Scan() {
		lines++
	}
	if lines >= 1000 || frozen.sc.Err() != nil {
		t.Fatalf("the client that did not read was not dropped: %d lines, %v", lines, frozen.sc.Err())
	}
}

func TestAnEventForAClientThatJustLeftIsDroppedNotACrash(t *testing.T) {
	dropped := 0
	q := &queue{lines: make(chan []byte, 1), drop: func() { dropped++ }}
	if _, err := q.Write([]byte("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Write([]byte("b")); err == nil || dropped != 1 {
		t.Fatalf("a full queue: err = %v, dropped %d", err, dropped)
	}
	q.close()
	q.close()
	if _, err := q.Write([]byte("c")); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("a write after the client left: %v", err)
	}
	if b := <-q.lines; string(b) != "a" {
		t.Fatalf("the queue lost what it held: %q", b)
	}
}
