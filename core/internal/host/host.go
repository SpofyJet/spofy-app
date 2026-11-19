package host

import (
	"errors"
	"os"
	"sync"
	"time"
)

var ErrNoPlatform = errors.New("host: no platform is attached")

type Spec struct {
	Name      string   `json:"name,omitempty"`
	MTU       int      `json:"mtu"`
	Addresses []string `json:"addresses"`
	Routes    []string `json:"routes"`
	DNS       []string `json:"dns"`
}

type Ask struct {
	N   uint64 `json:"ask"`
	Op  string `json:"op"`
	Tun *Spec  `json:"tun,omitempty"`
}

type Answer struct {
	OK    bool
	Error string
	File  *os.File
}

type Platform struct {
	mu      sync.Mutex
	send    func(Ask)
	owner   int
	next    uint64
	waiting map[uint64]chan Answer
}

func (p *Platform) Attach(send func(Ask)) (detach func()) {
	p.mu.Lock()
	p.owner++
	owner := p.owner
	p.send = send
	p.mu.Unlock()
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.owner != owner {
			return
		}
		p.send = nil
		for n, ch := range p.waiting {
			ch <- Answer{Error: "the platform is gone"}
			delete(p.waiting, n)
		}
	}
}

func (p *Platform) Answer(n uint64, a Answer) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch, ok := p.waiting[n]
	if !ok {
		if a.File != nil {
			a.File.Close()
		}
		return false
	}
	delete(p.waiting, n)
	ch <- a
	return true
}

func (p *Platform) Ask(op string, spec *Spec, wait time.Duration) (Answer, error) {
	p.mu.Lock()
	send := p.send
	if send == nil {
		p.mu.Unlock()
		return Answer{}, ErrNoPlatform
	}
	p.next++
	n := p.next
	ch := make(chan Answer, 1)
	if p.waiting == nil {
		p.waiting = map[uint64]chan Answer{}
	}
	p.waiting[n] = ch
	p.mu.Unlock()
	send(Ask{N: n, Op: op, Tun: spec})
	timer := time.NewTimer(wait)
	defer timer.Stop()
	var a Answer
	select {
	case a = <-ch:
	case <-timer.C:
		p.mu.Lock()
		delete(p.waiting, n)
		select {
		case a = <-ch:
		default:
			a = Answer{Error: "the platform did not answer"}
		}
		p.mu.Unlock()
	}
	if !a.OK {
		if a.File != nil {
			a.File.Close()
		}
		return Answer{}, errors.New("host: " + a.Error)
	}
	return a, nil
}

type Tun struct {
	Platform *Platform
	Spec     Spec
	Wait     time.Duration

	mu   sync.Mutex
	file *os.File
}

func (t *Tun) wait() time.Duration {
	if t.Wait > 0 {
		return t.Wait
	}
	return 10 * time.Second
}

func (t *Tun) Raise() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.file != nil {
		return nil
	}
	a, err := t.Platform.Ask("tun", &t.Spec, t.wait())
	if err != nil {
		return err
	}
	if a.File == nil {
		return errors.New("host: the platform sent no tunnel")
	}
	t.file = a.File
	return nil
}

func (t *Tun) Lower() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.file == nil {
		return nil
	}
	t.file.Close()
	t.file = nil
	if _, err := t.Platform.Ask("untun", nil, t.wait()); err != nil && !errors.Is(err, ErrNoPlatform) {
		return err
	}
	return nil
}

func (t *Tun) Up() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.file != nil
}

func (t *Tun) File() *os.File {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.file
}
