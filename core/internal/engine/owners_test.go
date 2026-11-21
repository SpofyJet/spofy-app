package engine

import (
	"testing"
	"time"

	"github.com/xtls/xray-core/common/net"

	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
)

func TestOwnersAskTheHostAndRefuseWithoutAnAnswer(t *testing.T) {
	asks := make(chan Reply, 4)
	var o owners
	o.open(func(r Reply) { asks <- r })
	src := net.TCPDestination(net.ParseAddress("172.19.0.1"), 40000)
	dst := net.TCPDestination(net.ParseAddress("2001:db8::1"), 443)
	type result struct {
		ok  bool
		out string
	}
	verdicts := make(chan result, 1)
	ask := func(network net.Network, dst net.Destination) {
		go func() {
			ok, out := o.ask(network, src, dst)
			verdicts <- result{ok, out}
		}()
	}
	ask(net.Network_TCP, dst)
	r := <-asks
	if r.Ev != "owner" || r.Ask == 0 || r.Flow == nil || *r.Flow != (Flow{Network: "tcp", Source: "172.19.0.1:40000", Destination: "[2001:db8::1]:443"}) {
		t.Fatalf("asked %+v %+v", r, r.Flow)
	}
	o.answer(r.Ask+1, true)
	o.answer(r.Ask, true)
	if v := <-verdicts; v != (result{true, ""}) {
		t.Fatalf("the answer of the host was not taken: %+v", v)
	}
	for _, c := range []struct {
		network net.Network
		dst     net.Destination
		want    result
	}{
		{net.Network_TCP, dst, result{false, ""}},
		{net.Network_UDP, net.UDPDestination(net.ParseAddress("172.19.0.2"), 53), result{true, xcfg.TagOutsideDNS}},
		{net.Network_TCP, net.TCPDestination(net.ParseAddress("172.19.0.2"), 53), result{true, xcfg.TagOutsideDNS}},
		{net.Network_TCP, net.TCPDestination(net.ParseAddress("8.8.8.8"), 853), result{true, xcfg.TagOutside}},
		{net.Network_TCP, net.TCPDestination(net.ParseAddress("2001:4860:4860::8888"), 853), result{true, xcfg.TagOutside}},
		{net.Network_TCP, net.TCPDestination(net.ParseAddress("172.19.0.2"), 853), result{false, ""}},
		{net.Network_TCP, net.TCPDestination(net.ParseAddress("::ffff:192.168.1.1"), 853), result{false, ""}},
		{net.Network_TCP, net.TCPDestination(net.ParseAddress("127.0.0.1"), 853), result{false, ""}},
		{net.Network_UDP, net.UDPDestination(net.ParseAddress("8.8.8.8"), 853), result{false, ""}},
		{net.Network_TCP, net.TCPDestination(net.ParseAddress("8.8.8.8"), 443), result{false, ""}},
	} {
		ask(c.network, c.dst)
		o.answer((<-asks).Ask, false)
		if v := <-verdicts; v != c.want {
			t.Fatalf("%v %v outside the VPN: %+v, want %+v", c.network, c.dst, v, c.want)
		}
	}
	began := time.Now()
	ask(net.Network_UDP, net.UDPDestination(net.ParseAddress("172.19.0.2"), 53))
	<-asks
	if v := <-verdicts; v != (result{}) || time.Since(began) < ownerWait {
		t.Fatalf("no answer must refuse after %v, got %+v after %v", ownerWait, v, time.Since(began))
	}
	ask(net.Network_UDP, net.UDPDestination(net.ParseAddress("172.19.0.2"), 53))
	<-asks
	began = time.Now()
	o.shut()
	if v := <-verdicts; v != (result{}) || time.Since(began) > time.Second {
		t.Fatalf("closing must refuse the connections that wait at once: %+v", v)
	}
	if ok, _ := o.ask(net.Network_UDP, src, net.UDPDestination(net.ParseAddress("172.19.0.2"), 53)); ok {
		t.Fatal("a closed check let a connection through")
	}
	if len(asks) != 0 {
		t.Fatal("a closed check still asks the host")
	}
}
