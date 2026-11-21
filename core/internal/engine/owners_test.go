package engine

import (
	"testing"
	"time"

	"github.com/xtls/xray-core/common/net"
)

func TestOwnersAskTheHostAndRefuseWithoutAnAnswer(t *testing.T) {
	asks := make(chan Reply, 4)
	var o owners
	o.open(func(r Reply) { asks <- r })
	src := net.TCPDestination(net.ParseAddress("172.19.0.1"), 40000)
	dst := net.TCPDestination(net.ParseAddress("2001:db8::1"), 443)
	verdict := make(chan bool, 1)
	go func() { verdict <- o.ask(net.Network_TCP, src, dst) }()
	r := <-asks
	if r.Ev != "owner" || r.Ask == 0 || r.Flow == nil || *r.Flow != (Flow{Network: "tcp", Source: "172.19.0.1:40000", Destination: "[2001:db8::1]:443"}) {
		t.Fatalf("asked %+v %+v", r, r.Flow)
	}
	o.answer(r.Ask+1, true)
	o.answer(r.Ask, true)
	if !<-verdict {
		t.Fatal("the answer of the host was not taken")
	}
	go func() { verdict <- o.ask(net.Network_UDP, src, dst) }()
	r = <-asks
	o.answer(r.Ask, false)
	if <-verdict {
		t.Fatal("a refusal of the host let the connection through")
	}
	began := time.Now()
	go func() { verdict <- o.ask(net.Network_TCP, src, dst) }()
	<-asks
	if <-verdict || time.Since(began) < ownerWait {
		t.Fatalf("no answer must refuse after %v, took %v", ownerWait, time.Since(began))
	}
	go func() { verdict <- o.ask(net.Network_TCP, src, dst) }()
	<-asks
	began = time.Now()
	o.shut()
	if <-verdict || time.Since(began) > time.Second {
		t.Fatal("closing must refuse the connections that wait at once")
	}
	if o.ask(net.Network_TCP, src, dst) {
		t.Fatal("a closed check let a connection through")
	}
	if len(asks) != 0 {
		t.Fatal("a closed check still asks the host")
	}
}
