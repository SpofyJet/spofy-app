package engine

import (
	"sync"
	"time"

	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/proxy/tun"
)

type Flow struct {
	Network     string `json:"network"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

const ownerWait = 2 * time.Second

type owners struct {
	mu      sync.Mutex
	send    func(Reply)
	next    uint64
	waiting map[uint64]chan bool
}

func (o *owners) open(send func(Reply)) {
	o.mu.Lock()
	o.send, o.waiting = send, map[uint64]chan bool{}
	o.mu.Unlock()
	tun.SetAdmit(o.ask)
}

func (o *owners) shut() {
	tun.SetAdmit(nil)
	o.mu.Lock()
	defer o.mu.Unlock()
	for n, ch := range o.waiting {
		ch <- false
		delete(o.waiting, n)
	}
	o.waiting = nil
}

func (o *owners) ask(network net.Network, source, destination net.Destination) bool {
	o.mu.Lock()
	if o.waiting == nil {
		o.mu.Unlock()
		return false
	}
	o.next++
	n, ch, send := o.next, make(chan bool, 1), o.send
	o.waiting[n] = ch
	o.mu.Unlock()
	send(Reply{Ev: "owner", Ask: n, Flow: &Flow{Network: network.SystemString(), Source: source.NetAddr(), Destination: destination.NetAddr()}})
	timer := time.NewTimer(ownerWait)
	defer timer.Stop()
	select {
	case in := <-ch:
		return in
	case <-timer.C:
		o.mu.Lock()
		delete(o.waiting, n)
		o.mu.Unlock()
		return false
	}
}

func (o *owners) answer(n uint64, in bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if ch, ok := o.waiting[n]; ok {
		delete(o.waiting, n)
		ch <- in
	}
}
