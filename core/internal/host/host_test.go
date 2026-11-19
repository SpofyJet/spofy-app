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
	_, err := f.Stat()
	return errors.Is(err, os.ErrClosed)
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
	})
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

func TestPlatformRefusalSilenceAndLoss(t *testing.T) {
	p := &Platform{}
	tun := &Tun{Platform: p, Wait: 50 * time.Millisecond}
	p.Attach(func(a Ask) { go p.Answer(a.N, Answer{Error: "the user said no"}) })
	if err := tun.Raise(); err == nil || !strings.Contains(err.Error(), "the user said no") || tun.Up() {
		t.Fatalf("refusal: %v", err)
	}
	p.Attach(func(a Ask) { go p.Answer(a.N, Answer{OK: true}) })
	if err := tun.Raise(); err == nil || tun.Up() {
		t.Fatalf("an answer without a tunnel: %v", err)
	}
	var late Ask
	p.Attach(func(a Ask) { late = a })
	if err := tun.Raise(); err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("silence: %v", err)
	}
	file := pipe(t)
	if p.Answer(late.N, Answer{OK: true, File: file}) || !closed(file) {
		t.Fatal("a late answer was accepted or its descriptor was left open")
	}
	slow := &Tun{Platform: p, Wait: 5 * time.Second}
	detach := p.Attach(func(Ask) {})
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
	stale := p.Attach(func(Ask) {})
	p.Attach(func(a Ask) { go p.Answer(a.N, Answer{OK: true, File: pipe(t)}) })
	stale()
	if err := tun.Raise(); err != nil {
		t.Fatalf("an old detach removed the new platform: %v", err)
	}
}
