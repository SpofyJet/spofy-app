//go:build unix

package ipc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/host"
)

func listenChannel(t *testing.T) (net.Listener, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "core.sock")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path})
	if err != nil {
		t.Fatal(err)
	}
	return l, path
}

func dialChannel(path string) (net.Conn, error) {
	return net.DialUnix("unix", nil, &net.UnixAddr{Name: path})
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

	service.send(`{"id":2,"op":"platform","owners":true}`)
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
	if err := host.Send(intruder.conn.(*net.UnixConn), []byte(fmt.Sprintf(`{"answer":%v,"ok":true,"file":true}`+"\n", m["ask"])), stray); err != nil {
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
	if err := host.Send(service.conn.(*net.UnixConn), []byte(fmt.Sprintf(`{"answer":%v,"ok":true,"file":true}`+"\n", m["ask"])), tun); err != nil {
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

	if !p.Knows() {
		t.Fatal("the platform said it names the owners of connections")
	}
	inside := make(chan bool, 1)
	go func() { inside <- p.Inside("tcp", "172.19.0.1:40000", "[2001:db8::1]:443") }()
	m = service.until(func(m map[string]any) bool { return m["ask"] != nil })
	flow, _ := m["flow"].(map[string]any)
	if m["op"] != "owner" || flow["network"] != "tcp" || flow["source"] != "172.19.0.1:40000" || flow["destination"] != "[2001:db8::1]:443" {
		t.Fatalf("owner ask: %v", m)
	}
	service.send(fmt.Sprintf(`{"answer":%v,"ok":true,"in":true}`, m["ask"]))
	if !<-inside {
		t.Fatal("the verdict of the platform was lost on the way")
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
