//go:build unix

package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/host"
)

type client struct {
	t    *testing.T
	conn *net.UnixConn
	sc   *bufio.Scanner
}

func dial(t *testing.T, path string) *client {
	t.Helper()
	c, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path})
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
	path := filepath.Join(t.TempDir(), "core.sock")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path})
	if err != nil {
		t.Fatal(err)
	}
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

func TestSocketServesTheInterfaceAndThePlatform(t *testing.T) {
	r := newRig(t)
	p := &host.Platform{}
	path, cancel, done := listen(t, r, p)
	ui, service := dial(t, path), dial(t, path)

	ui.send(`{"id":1,"op":"hello"}`)
	if m := ui.reply(1); m["ok"] != true || m["snapshot"] == nil {
		t.Fatalf("hello: %v", m)
	}
	ui.send(`{"id":2,"op":"platform"}`)
	service.send(`{"id":1,"op":"set","key":"theme","value":"dark"}`)
	seen := func(m map[string]any) bool { return m["ev"] == "settings" }
	ui.until(seen)
	service.until(seen)
	if _, err := p.Ask("tun", &host.Spec{MTU: 1500}, 50*time.Millisecond); err == nil {
		t.Fatal("a connection that did not ask for the role answered")
	}
	ui.conn.Close()

	service.send(`{"id":2,"op":"platform"}`)
	if m := service.reply(2); m["ok"] != true {
		t.Fatalf("platform: %v", m)
	}
	file := func() *os.File {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { r.Close(); w.Close() })
		return w
	}
	type result struct {
		a   host.Answer
		err error
	}
	ask := func(op string, spec *host.Spec) chan result {
		out := make(chan result, 1)
		go func() {
			a, err := p.Ask(op, spec, 5*time.Second)
			out <- result{a, err}
		}()
		return out
	}

	pending := ask("tun", &host.Spec{MTU: 1500, Addresses: []string{"172.19.0.1/30"}, Routes: []string{"0.0.0.0/0"}, DNS: []string{"172.19.0.2"}})
	m := service.until(func(m map[string]any) bool { return m["ask"] != nil })
	spec, _ := m["tun"].(map[string]any)
	if m["op"] != "tun" || spec["mtu"] != float64(1500) || fmt.Sprint(spec["routes"]) != "[0.0.0.0/0]" || fmt.Sprint(spec["dns"]) != "[172.19.0.2]" {
		t.Fatalf("ask: %v", m)
	}
	intruder := dial(t, path)
	stray := file()
	if err := host.Send(intruder.conn, []byte(fmt.Sprintf(`{"answer":%v,"ok":true,"file":true}`+"\n", m["ask"])), stray); err != nil {
		t.Fatal(err)
	}
	intruder.send(`{"id":1,"op":"hello"}`)
	intruder.reply(1)
	select {
	case got := <-pending:
		t.Fatalf("a connection without the role answered the ask: %+v", got)
	default:
	}
	tun := file()
	if err := host.Send(service.conn, []byte(fmt.Sprintf(`{"answer":%v,"ok":true,"file":true}`+"\n", m["ask"])), tun); err != nil {
		t.Fatal(err)
	}
	got := <-pending
	if got.err != nil || got.a.File == nil {
		t.Fatalf("answer with a descriptor: %+v", got)
	}
	if _, err := got.a.File.Write([]byte("x")); err != nil {
		t.Fatalf("the descriptor that came is not the pipe that was sent: %v", err)
	}
	got.a.File.Close()

	pending = ask("untun", nil)
	m = service.until(func(m map[string]any) bool { return m["ask"] != nil })
	service.send(fmt.Sprintf(`{"answer":%v,"ok":false,"error":"denied by the user"}`, m["ask"]))
	if got := <-pending; got.err == nil || !strings.Contains(got.err.Error(), "denied by the user") {
		t.Fatalf("refusal: %+v", got)
	}

	pending = ask("tun", &host.Spec{})
	service.until(func(m map[string]any) bool { return m["ask"] != nil })
	began := time.Now()
	service.conn.Close()
	if got := <-pending; got.err == nil || time.Since(began) > 2*time.Second {
		t.Fatalf("the platform left with an ask open: %+v after %v", got, time.Since(began))
	}
	if _, err := p.Ask("tun", &host.Spec{}, time.Second); err != host.ErrNoPlatform {
		t.Fatalf("ask without a platform: %v", err)
	}

	last := dial(t, path)
	last.send(`{"id":1,"op":"hello"}`)
	last.reply(1)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the listener did not stop")
	}
	last.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if last.sc.Scan() {
		t.Fatalf("a line after the listener stopped: %s", last.sc.Text())
	}
	done <- nil
}

func TestSocketClientThatDoesNotReadIsDropped(t *testing.T) {
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
	began := time.Now()
	count := 0
	seen := make(chan int, 1)
	go func() {
		n := 0
		live.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		for live.sc.Scan() {
			if strings.Contains(live.sc.Text(), `"ev":"settings"`) {
				n++
				if n == 1000 {
					break
				}
			}
		}
		seen <- n
	}()
	for ; count < 1000; count++ {
		if err := r.app.Set("note", json.RawMessage(value)); err != nil {
			t.Fatal(err)
		}
	}
	if took := time.Since(began); took > 20*time.Second {
		t.Fatalf("a client that does not read slowed the core down: %v", took)
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
