package host

import (
	"errors"
	"os"
	"sync"
	"time"
)

var ErrNoPlatform = errors.New("host: no platform is attached")

type Apps struct {
	Mode string   `json:"mode"`
	List []string `json:"list"`
}

type Spec struct {
	Name      string   `json:"name,omitempty"`
	MTU       int      `json:"mtu"`
	Addresses []string `json:"addresses"`
	Routes    []string `json:"routes"`
	DNS       []string `json:"dns"`
	Apps      *Apps    `json:"apps,omitempty"`
}

type Flow struct {
	Network     string `json:"network"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

type Ask struct {
	N    uint64 `json:"ask"`
	Op   string `json:"op"`
	Tun  *Spec  `json:"tun,omitempty"`
	Flow *Flow  `json:"flow,omitempty"`
}

type Answer struct {
	OK    bool
	Error string
	File  *os.File
	In    bool
}

const ownerWait = 1500 * time.Millisecond

type Platform struct {
	mu      sync.Mutex
	send    func(Ask)
	owners  bool
	owner   int
	next    uint64
	waiting map[uint64]chan Answer
}

func (p *Platform) Attach(send func(Ask), owners bool) (detach func()) {
	p.mu.Lock()
	p.owner++
	owner := p.owner
	p.send, p.owners = send, owners
	p.mu.Unlock()
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.owner != owner {
			return
		}
		p.send, p.owners = nil, false
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

func (p *Platform) Knows() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.send != nil && p.owners
}

func (p *Platform) Inside(network, source, destination string) bool {
	a, err := p.ask(Ask{Op: "owner", Flow: &Flow{Network: network, Source: source, Destination: destination}}, ownerWait)
	return err == nil && a.In
}

func (p *Platform) Ask(op string, spec *Spec, wait time.Duration) (Answer, error) {
	return p.ask(Ask{Op: op, Tun: spec}, wait)
}

func (p *Platform) ask(ask Ask, wait time.Duration) (Answer, error) {
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
	ask.N = n
	send(ask)
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
	Apps     func() *Apps
	Wait     time.Duration

	mu   sync.Mutex
	file *os.File
}

func (t *Tun) ask() (*os.File, error) {
	spec := t.Spec
	if t.Apps != nil {
		spec.Apps = t.Apps()
	}
	a, err := t.Platform.Ask("tun", &spec, t.wait())
	if err != nil {
		return nil, err
	}
	if a.File == nil {
		return nil, errors.New("host: the platform sent no tunnel")
	}
	return a.File, nil
}

func (t *Tun) Renew() (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.file == nil {
		return false, nil
	}
	file, err := t.ask()
	if err != nil {
		return false, err
	}
	t.file.Close()
	t.file = file
	return true, nil
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
	file, err := t.ask()
	if err != nil {
		return err
	}
	t.file = file
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
