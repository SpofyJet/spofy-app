package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
	"github.com/SpofyJet/spofy-app/core/internal/xcfg/xcfgtest"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func proxyConfig(t *testing.T, raw json.RawMessage) []byte {
	t.Helper()
	cfg, err := xcfgtest.Socks(raw, freePort(t), "")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func fixtureServers(t *testing.T, path string) []json.RawMessage {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Skip(err)
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(body, &raws); err != nil {
		t.Fatal(err)
	}
	return raws
}

func TestEveryFixtureServerLoads(t *testing.T) {
	for _, path := range []string{"../../testdata/sub/sub.json", "../../../testdata/private/sub.json"} {
		for i, raw := range fixtureServers(t, path) {
			plain := xcfg.Options{Mode: xcfg.ModeTun, Tun: xcfg.Tun{Name: "spofy0", MTU: 1500, Gateway: []string{"172.19.0.1/30"}, Routes: []string{"0.0.0.0/1", "128.0.0.0/1"}}}
			tweaked := plain
			tweaked.Tun.Resolver = []string{"172.19.0.2"}
			tweaked.Tweaks = xcfg.Tweaks{Fragment: true, DNS: []string{"1.1.1.1", "2606:4700:4700::1111"}, NoIPv6: true, LAN: true}
			for _, o := range []xcfg.Options{plain, tweaked, {Mode: xcfg.ModeProbe, Tweaks: xcfg.Tweaks{Fragment: true}}} {
				tun, err := xcfg.Build(raw, o)
				if err != nil {
					t.Fatalf("%s #%d: %v", path, i, err)
				}
				if err := Validate(tun); err != nil {
					t.Fatalf("%s #%d %+v: %v", path, i, o.Tweaks, err)
				}
			}
			if err := Validate(proxyConfig(t, raw)); err != nil {
				t.Fatalf("%s #%d proxy: %v", path, i, err)
			}
		}
	}
}

func TestStartStopProxyMode(t *testing.T) {
	raw := fixtureServers(t, "../../testdata/sub/sub.json")[0]
	port := freePort(t)
	cfg, err := xcfgtest.Socks(raw, port, "")
	if err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 3; round++ {
		inst, err := Start(cfg)
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
		if err != nil {
			t.Fatalf("round %d: inbound is not listening: %v", round, err)
		}
		c.Close()
		if got := inst.Traffic(xcfg.TagTun); got.Up != 0 || got.Down != 0 {
			t.Fatalf("round %d: traffic = %+v", round, got)
		}
		if err := inst.Close(); err != nil {
			t.Fatalf("round %d: close: %v", round, err)
		}
		if c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond); err == nil {
			c.Close()
			t.Fatalf("round %d: port still open after close", round)
		}
	}
}

func TestOnlyOutboundsThatLeadToAServerCountAsItsAnswers(t *testing.T) {
	got := far([]byte(`{"outbounds":[
		{"tag":"proxy","protocol":"vless"},{"tag":"direct","protocol":"freedom"},{"tag":"block","protocol":"blackhole"},
		{"tag":"dns-out","protocol":"dns"},{"protocol":"trojan"},{"tag":"second","protocol":"hysteria"},{"tag":"loop","protocol":"loopback"}]}`))
	if len(got) != 2 || got[0] != "proxy" || got[1] != "second" {
		t.Fatalf("far = %v", got)
	}
	if got := far([]byte(`{`)); got != nil {
		t.Fatalf("far of a broken config = %v", got)
	}
	inst, err := Start([]byte(probeConfig))
	if err != nil {
		t.Fatal(err)
	}
	defer inst.Close()
	if len(inst.far) != 0 {
		t.Fatalf("far = %v", inst.far)
	}
	srv := probeTarget(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := inst.Probe(ctx, srv.URL+"/generate_204", ""); err != nil {
		t.Fatal(err)
	}
	if tr := inst.Traffic("none"); tr.Back != tr.Down {
		t.Fatalf("with no outbound to a server the answers are what came into the tunnel: %+v", tr)
	}
}

func TestStartClassifiesErrors(t *testing.T) {
	_, err := Start([]byte(`{"outbounds":[{"protocol":"nope"}]}`))
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeConfig {
		t.Fatalf("err = %v", err)
	}
	if got := classify(errors.New("open /dev/net/tun: operation not permitted"), CodeStart); got.Code != CodePermission {
		t.Fatalf("code = %s", got.Code)
	}
	if got := classify(errors.New("Access is denied."), CodeStart); got.Code != CodePermission {
		t.Fatalf("code = %s", got.Code)
	}
	if got := classify(errors.New("address already in use"), CodeStart); got.Code != CodeStart {
		t.Fatalf("code = %s", got.Code)
	}
}

func TestServeProtocol(t *testing.T) {
	raw := fixtureServers(t, "../../testdata/sub/sub.json")[0]
	cfg := proxyConfig(t, raw)
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- Serve(inR, outW) }()
	lines := bufio.NewScanner(outR)
	lines.Buffer(make([]byte, 64<<10), maxLine)
	next := func() Reply {
		t.Helper()
		for lines.Scan() {
			var m Reply
			if err := json.Unmarshal(lines.Bytes(), &m); err != nil {
				t.Fatalf("bad line %q", lines.Text())
			}
			if m.Ev == "log" {
				continue
			}
			return m
		}
		t.Fatal("stream closed")
		return Reply{}
	}
	send := func(v any) {
		b, _ := json.Marshal(v)
		inW.Write(append(b, '\n'))
	}
	if m := next(); m.Ev != "ready" {
		t.Fatalf("first message = %+v", m)
	}
	send(Request{ID: 1, Op: "start", Config: cfg})
	if m := next(); m.Re != 1 || !m.OK {
		t.Fatalf("start reply = %+v", m)
	}
	send(Request{ID: 2, Op: "traffic", Tag: xcfg.TagTun})
	if m := next(); m.Re != 2 || !m.OK || m.Traffic == nil {
		t.Fatalf("traffic reply = %+v", m)
	}
	send(Request{ID: 3, Op: "start", Config: json.RawMessage(`{"outbounds":[{"protocol":"nope"}]}`)})
	if m := next(); m.Re != 3 || m.OK || m.Code != CodeConfig {
		t.Fatalf("bad start reply = %+v", m)
	}
	inW.Write([]byte("not json\n"))
	if m := next(); m.Ev != "error" {
		t.Fatalf("garbage reply = %+v", m)
	}
	send(Request{ID: 4, Op: "start", Config: cfg})
	if m := next(); m.Re != 4 || !m.OK {
		t.Fatalf("restart reply = %+v", m)
	}
	send(Request{ID: 5, Op: "stop"})
	if m := next(); m.Re != 5 || !m.OK {
		t.Fatalf("stop reply = %+v", m)
	}
	inW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not exit after input closed")
	}
}

func TestLogsGoToSinkNotStdout(t *testing.T) {
	raw := fixtureServers(t, "../../testdata/sub/sub.json")[0]
	cfg, err := xcfgtest.Socks(raw, freePort(t), "info")
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan string, 256)
	SetLogSink(func(line string) {
		select {
		case got <- line:
		default:
		}
	})
	defer SetLogSink(nil)
	inst, err := Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer inst.Close()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case line := <-got:
			if strings.Contains(line, "started") || strings.Contains(line, "Xray") {
				return
			}
		case <-deadline:
			t.Fatal("no core log line reached the sink")
		}
	}
}
