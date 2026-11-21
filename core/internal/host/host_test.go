package host

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func pipe(t *testing.T) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })
	return r
}

func closed(f *os.File) bool {
	return f.Fd() == ^uintptr(0)
}

func TestTunAsksThePlatformOnceAndGivesTheDescriptorToTheEngine(t *testing.T) {
	p := &Platform{}
	tun := &Tun{Platform: p, Spec: Spec{MTU: 1500, Addresses: []string{"172.19.0.1/30"}}, Wait: time.Second}
	if err := tun.Raise(); !errors.Is(err, ErrNoPlatform) {
		t.Fatalf("without a platform: %v", err)
	}
	file := pipe(t)
	var asked []Ask
	detach := p.Attach(func(a Ask) {
		asked = append(asked, a)
		go p.Answer(a.N, Answer{OK: true, File: map[string]*os.File{"tun": file}[a.Op]})
	}, false)
	for range 2 {
		if err := tun.Raise(); err != nil {
			t.Fatal(err)
		}
	}
	if len(asked) != 1 || asked[0].Op != "tun" || asked[0].Tun.MTU != 1500 || !tun.Up() {
		t.Fatalf("asked %+v, up %v", asked, tun.Up())
	}
	if tun.File() != file {
		t.Fatalf("file %v", tun.File())
	}
	if err := tun.Lower(); err != nil || tun.Up() || !closed(file) {
		t.Fatalf("lower: err %v, up %v, closed %v", err, tun.Up(), closed(file))
	}
	if len(asked) != 2 || asked[1].Op != "untun" {
		t.Fatalf("asked %+v", asked)
	}
	if tun.File() != nil {
		t.Fatalf("a lowered tunnel still has a file: %v", tun.File())
	}
	detach()
	if err := tun.Lower(); err != nil {
		t.Fatalf("lower twice: %v", err)
	}
}

func TestRenewedTunnelCarriesTheChosenAppsAndReplacesTheOldOne(t *testing.T) {
	p := &Platform{}
	chosen := &Apps{Mode: "except", List: []string{"org.example.one"}}
	tun := &Tun{Platform: p, Spec: Spec{MTU: 1500}, Apps: func() *Apps { return chosen }, Wait: time.Second}
	first, second := pipe(t), pipe(t)
	files := []*os.File{first, second}
	var asked []Ask
	p.Attach(func(a Ask) {
		asked = append(asked, a)
		if a.Op != "tun" {
			go p.Answer(a.N, Answer{OK: true})
			return
		}
		file := files[0]
		files = files[1:]
		go p.Answer(a.N, Answer{OK: true, File: file})
	}, false)
	if again, err := tun.Renew(); again || err != nil || len(asked) != 0 {
		t.Fatalf("a tunnel that is not up was renewed: %v %v, asked %d", again, err, len(asked))
	}
	if err := tun.Raise(); err != nil {
		t.Fatal(err)
	}
	if got := asked[0].Tun.Apps; got == nil || got.Mode != "except" || len(got.List) != 1 {
		t.Fatalf("the request does not carry the apps: %+v", asked[0].Tun)
	}
	chosen = &Apps{Mode: "only", List: []string{"org.example.two"}}
	if again, err := tun.Renew(); !again || err != nil {
		t.Fatalf("renew: %v %v", again, err)
	}
	if len(asked) != 2 || asked[1].Op != "tun" || asked[1].Tun.Apps.Mode != "only" || tun.File() != second || !closed(first) || closed(second) {
		t.Fatalf("asked %d, file is the new one %v, old closed %v", len(asked), tun.File() == second, closed(first))
	}
	if tun.Spec.Apps != nil {
		t.Fatal("the shared description was changed by a request")
	}
	chosen = nil
	p.Attach(func(a Ask) { go p.Answer(a.N, Answer{Error: "no"}) }, false)
	if again, err := tun.Renew(); again || err == nil || tun.File() != second || closed(second) {
		t.Fatalf("a refused renewal dropped the working tunnel: %v %v", again, err)
	}
}

func TestPlatformRefusalSilenceAndLoss(t *testing.T) {
	p := &Platform{}
	tun := &Tun{Platform: p, Wait: 50 * time.Millisecond}
	p.Attach(func(a Ask) { go p.Answer(a.N, Answer{Error: "the user said no"}) }, false)
	if err := tun.Raise(); err == nil || !strings.Contains(err.Error(), "the user said no") || tun.Up() {
		t.Fatalf("refusal: %v", err)
	}
	p.Attach(func(a Ask) { go p.Answer(a.N, Answer{OK: true}) }, false)
	if err := tun.Raise(); err == nil || tun.Up() {
		t.Fatalf("an answer without a tunnel: %v", err)
	}
	var late Ask
	p.Attach(func(a Ask) { late = a }, false)
	if err := tun.Raise(); err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("silence: %v", err)
	}
	file := pipe(t)
	if p.Answer(late.N, Answer{OK: true, File: file}) || !closed(file) {
		t.Fatal("a late answer was accepted or its descriptor was left open")
	}
	slow := &Tun{Platform: p, Wait: 5 * time.Second}
	detach := p.Attach(func(Ask) {}, false)
	done := make(chan error, 1)
	go func() { done <- slow.Raise() }()
	time.Sleep(20 * time.Millisecond)
	detach()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "gone") {
			t.Fatalf("loss: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a lost platform left the request waiting")
	}
	stale := p.Attach(func(Ask) {}, false)
	p.Attach(func(a Ask) { go p.Answer(a.N, Answer{OK: true, File: pipe(t)}) }, false)
	stale()
	if err := tun.Raise(); err != nil {
		t.Fatalf("an old detach removed the new platform: %v", err)
	}
}

func TestPlatformNamesTheOwnerOnlyWhenItSaidItCan(t *testing.T) {
	p := &Platform{}
	if p.Knows() || p.Inside("tcp", "172.19.0.1:40000", "1.2.3.4:443") {
		t.Fatal("no platform knows the owner of a connection")
	}
	var asked []Ask
	detach := p.Attach(func(a Ask) {
		asked = append(asked, a)
		go p.Answer(a.N, Answer{OK: true, In: a.Flow.Source == "172.19.0.1:40000"})
	}, true)
	if !p.Knows() || !p.Inside("tcp", "172.19.0.1:40000", "1.2.3.4:443") || p.Inside("udp", "172.19.0.1:40001", "1.2.3.4:443") {
		t.Fatal("the verdict of the platform was not taken")
	}
	if len(asked) != 2 || asked[0].Op != "owner" || asked[0].Tun != nil || *asked[1].Flow != (Flow{Network: "udp", Source: "172.19.0.1:40001", Destination: "1.2.3.4:443"}) {
		t.Fatalf("asked %+v", asked)
	}
	detach()
	if p.Knows() || p.Inside("tcp", "172.19.0.1:40000", "1.2.3.4:443") {
		t.Fatal("a platform that is gone still lets connections in")
	}
	p.Attach(func(a Ask) { go p.Answer(a.N, Answer{OK: true, In: true}) }, false)
	if p.Knows() {
		t.Fatal("a platform that cannot name owners is asked")
	}
	began := time.Now()
	p.Attach(func(Ask) {}, true)
	if p.Inside("tcp", "172.19.0.1:40000", "1.2.3.4:443") || time.Since(began) < ownerWait || time.Since(began) > ownerWait+time.Second {
		t.Fatalf("a silent platform must refuse after %v, took %v", ownerWait, time.Since(began))
	}
}
