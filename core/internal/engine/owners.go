package engine

import (
	"net/netip"
	"sync"
	"time"

	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/proxy/tun"

	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
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
	waiting map[uint64]chan verdict
}

type verdict struct{ in, closed bool }

func (o *owners) open(send func(Reply)) {
	o.mu.Lock()
	o.send, o.waiting = send, map[uint64]chan verdict{}
	o.mu.Unlock()
	tun.SetAdmit(o.ask)
}

func (o *owners) shut() {
	tun.SetAdmit(nil)
	o.mu.Lock()
	defer o.mu.Unlock()
	for n, ch := range o.waiting {
		ch <- verdict{closed: true}
		delete(o.waiting, n)
	}
	o.waiting = nil
}

func (o *owners) ask(network net.Network, source, destination net.Destination) (bool, string) {
	o.mu.Lock()
	if o.waiting == nil {
		o.mu.Unlock()
		return false, ""
	}
	o.next++
	n, ch, send := o.next, make(chan verdict, 1), o.send
	o.waiting[n] = ch
	o.mu.Unlock()
	send(Reply{Ev: "owner", Ask: n, Flow: &Flow{Network: network.SystemString(), Source: source.NetAddr(), Destination: destination.NetAddr()}})
	timer := time.NewTimer(ownerWait)
	defer timer.Stop()
	select {
	case answer := <-ch:
		if answer.closed || answer.in {
			return answer.in, ""
		}
		return past(network, destination)
	case <-timer.C:
		o.mu.Lock()
		delete(o.waiting, n)
		o.mu.Unlock()
		return false, ""
	}
}

func past(network net.Network, destination net.Destination) (bool, string) {
	if destination.Port == 53 {
		return true, xcfg.TagOutsideDNS
	}
	if network != net.Network_TCP || destination.Port != 853 || !destination.Address.Family().IsIP() {
		return false, ""
	}
	ip, ok := netip.AddrFromSlice(destination.Address.IP())
	if ip = ip.Unmap(); !ok || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false, ""
	}
	return true, xcfg.TagOutside
}

func (o *owners) answer(n uint64, in bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if ch, ok := o.waiting[n]; ok {
		delete(o.waiting, n)
		ch <- verdict{in: in}
	}
}
