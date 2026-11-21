package xcfg

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func servers(t *testing.T, path string) []json.RawMessage {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(body, &raws); err != nil {
		t.Fatal(err)
	}
	return raws
}

var tunOptions = Options{Mode: ModeTun, Tun: Tun{
	Name: "spofy0", MTU: 1500, Gateway: []string{"172.19.0.1/30"},
	Routes: []string{"0.0.0.0/1", "128.0.0.0/1"},
}}

func TestBuildTunKeepsServerLogic(t *testing.T) {
	for i, raw := range servers(t, "../../testdata/sub/sub.json") {
		built, err := Build(raw, tunOptions)
		if err != nil {
			t.Fatalf("server %d: %v", i, err)
		}
		var in, out map[string]json.RawMessage
		json.Unmarshal(raw, &in)
		if err := json.Unmarshal(built, &out); err != nil {
			t.Fatalf("server %d: %v", i, err)
		}
		for _, k := range []string{"outbounds", "dns", "routing", "fakedns", "burstObservatory"} {
			if !bytes.Equal(in[k], out[k]) && !(onlyIPv4(in["dns"]) && sameButIPv6Block(in[k], out[k])) {
				t.Fatalf("server %d: section %s changed", i, k)
			}
		}
		if _, ok := out["remarks"]; ok {
			t.Fatalf("server %d: remarks kept", i)
		}
		var inbounds []struct {
			Tag      string          `json:"tag"`
			Protocol string          `json:"protocol"`
			Listen   string          `json:"listen"`
			Sniffing json.RawMessage `json:"sniffing"`
			Settings struct {
				Name   string   `json:"name"`
				Routes []string `json:"autoSystemRoutingTable"`
			} `json:"settings"`
		}
		json.Unmarshal(out["inbounds"], &inbounds)
		if len(inbounds) != 1 || inbounds[0].Protocol != "tun" || inbounds[0].Tag != TagTun || inbounds[0].Listen != "" {
			t.Fatalf("server %d: inbounds = %s", i, out["inbounds"])
		}
		if inbounds[0].Settings.Name != "spofy0" || len(inbounds[0].Settings.Routes) != 2 {
			t.Fatalf("server %d: tun settings = %s", i, out["inbounds"])
		}
		var orig []struct {
			Sniffing json.RawMessage `json:"sniffing"`
		}
		json.Unmarshal(in["inbounds"], &orig)
		var was, now map[string]any
		json.Unmarshal(orig[0].Sniffing, &was)
		json.Unmarshal(inbounds[0].Sniffing, &now)
		excluded, _ := now["ipsExcluded"].([]any)
		delete(now, "ipsExcluded")
		if !reflect.DeepEqual(was, now) || len(excluded) != len(nearby) || excluded[2] != "192.168.0.0/16" {
			t.Fatalf("server %d: sniffing = %s", i, inbounds[0].Sniffing)
		}
		if string(out["log"]) != `{"access":"none","loglevel":"warning"}` {
			t.Fatalf("server %d: log = %s", i, out["log"])
		}
	}
}

func sameButIPv6Block(in, out json.RawMessage) bool {
	var a, b any
	if json.Unmarshal(in, &a) != nil || json.Unmarshal(out, &b) != nil {
		return false
	}
	return reflect.DeepEqual(withoutIPv6Block(a), withoutIPv6Block(b))
}

func withoutIPv6Block(v any) any {
	if m, ok := v.(map[string]any); ok && m["rules"] != nil {
		m["rules"] = withoutIPv6Block(m["rules"])
		return m
	}
	list, ok := v.([]any)
	if !ok {
		return v
	}
	var kept []any
	for _, e := range list {
		m, ok := e.(map[string]any)
		if ok && (m["tag"] == TagBlock || m["outboundTag"] == TagBlock) {
			continue
		}
		if s, ok := m["settings"].(map[string]any); ok && m["protocol"] == "dns" {
			delete(s, "blockTypes")
		}
		kept = append(kept, e)
	}
	return kept
}

func TestBuildTunGivesTheResolverAddressToTheNameHandlerAndRefusesTheRest(t *testing.T) {
	type shape struct {
		Outbounds []struct {
			Tag      string `json:"tag"`
			Protocol string `json:"protocol"`
			Settings struct {
				Redirect string `json:"redirect"`
			} `json:"settings"`
		} `json:"outbounds"`
		Routing struct {
			Strategy string `json:"domainStrategy"`
			Rules    []struct {
				In   []string `json:"inboundTag"`
				IP   []string `json:"ip"`
				Port string   `json:"port"`
				Out  string   `json:"outboundTag"`
			} `json:"rules"`
		} `json:"routing"`
	}
	options := Options{Mode: ModeTun, Tun: Tun{Name: "spofy0", MTU: 1500, Resolver: []string{"172.19.0.2"}}}
	build := func(server string) shape {
		t.Helper()
		built, err := Build(json.RawMessage(server), options)
		if err != nil {
			t.Fatal(err)
		}
		var s shape
		if err := json.Unmarshal(built, &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	bare := build(`{"outbounds":[{"tag":"proxy","protocol":"vless"}]}`)
	if n := len(bare.Outbounds); n != 5 || bare.Outbounds[0].Tag != "proxy" || bare.Outbounds[1].Protocol != "dns" || bare.Outbounds[2].Protocol != "blackhole" ||
		bare.Outbounds[3].Tag != TagOutside || bare.Outbounds[3].Protocol != "freedom" || bare.Outbounds[4].Tag != TagOutsideDNS || bare.Outbounds[4].Settings.Redirect != outsideDNS {
		t.Fatalf("outbounds = %+v", bare.Outbounds)
	}
	if r := bare.Routing.Rules; len(r) != 2 || r[0].Port != "53" || r[0].Out != bare.Outbounds[1].Tag || r[0].IP[0] != "172.19.0.2" || r[0].In[0] != TagTun ||
		r[1].Port != "" || r[1].Out != bare.Outbounds[2].Tag || r[1].IP[0] != "172.19.0.2" {
		t.Fatalf("rules = %+v", r)
	}
	full := build(`{"outbounds":[{"tag":"proxy","protocol":"vless"},{"tag":"names","protocol":"dns"},{"tag":"none","protocol":"blackhole"}],
		"routing":{"domainStrategy":"IPIfNonMatch","rules":[{"type":"field","port":"53","outboundTag":"names"}]}}`)
	if n := len(full.Outbounds); n != 5 {
		t.Fatalf("outbounds = %+v", full.Outbounds)
	}
	if r := full.Routing.Rules; len(r) != 3 || r[0].Out != "names" || r[1].Out != "none" || r[2].Out != "names" || r[2].IP != nil || full.Routing.Strategy != "IPIfNonMatch" {
		t.Fatalf("rules = %+v", r)
	}
	probe, err := Build(json.RawMessage(`{"outbounds":[{"tag":"proxy","protocol":"vless"}]}`), Options{Mode: ModeProbe, Tun: options.Tun})
	if err != nil || bytes.Contains(probe, []byte("172.19.0.2")) {
		t.Fatalf("probe config: %v %s", err, probe)
	}
}

func TestNoServerOpensAWayIn(t *testing.T) {
	body, err := os.ReadFile("../../testdata/sub/sub.json")
	if err != nil {
		t.Fatal(err)
	}
	var servers []json.RawMessage
	if err := json.Unmarshal(body, &servers); err != nil || len(servers) == 0 {
		t.Fatal("no servers in the sample", err)
	}
	for i, server := range servers {
		for _, o := range []Options{tunOptions, {Mode: ModeProbe}} {
			built, err := Build(server, o)
			if err != nil {
				t.Fatalf("#%d: %v", i, err)
			}
			var out struct {
				API      json.RawMessage `json:"api"`
				Metrics  json.RawMessage `json:"metrics"`
				Inbounds []struct {
					Protocol string          `json:"protocol"`
					Listen   json.RawMessage `json:"listen"`
					Port     json.RawMessage `json:"port"`
				} `json:"inbounds"`
			}
			if err := json.Unmarshal(built, &out); err != nil {
				t.Fatal(err)
			}
			if out.API != nil || out.Metrics != nil {
				t.Fatalf("#%d mode %d: a control port is kept: %s", i, o.Mode, built)
			}
			for _, in := range out.Inbounds {
				if in.Protocol != "tun" || in.Listen != nil || (in.Port != nil && string(in.Port) != "0") {
					t.Fatalf("#%d mode %d: a way in that other programs can reach: %s", i, o.Mode, built)
				}
			}
		}
	}
}

func TestBuildDropsDangerousSections(t *testing.T) {
	raw := json.RawMessage(`{
		"api":{"tag":"api","listen":"0.0.0.0:10813"},
		"metrics":{"listen":"0.0.0.0:9999"},
		"log":{"access":"/etc/passwd","loglevel":"debug"},
		"reverse":{},
		"inbounds":[{"tag":"api-in","protocol":"dokodemo-door","listen":"0.0.0.0","port":10813}],
		"outbounds":[{"tag":"proxy","protocol":"freedom"}],
		"routing":{"rules":[{"type":"field","inboundTag":["api-in","other"],"outboundTag":"proxy"}]}
	}`)
	built, err := Build(raw, tunOptions)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	json.Unmarshal(built, &out)
	for _, k := range []string{"api", "metrics", "reverse"} {
		if _, ok := out[k]; ok {
			t.Fatalf("%s kept", k)
		}
	}
	if bytes.Contains(built, []byte("/etc/passwd")) || bytes.Contains(built, []byte("10813")) {
		t.Fatalf("dangerous values kept: %s", built)
	}
	var routing struct {
		Rules []struct {
			InboundTag []string `json:"inboundTag"`
		} `json:"rules"`
	}
	json.Unmarshal(out["routing"], &routing)
	if got := routing.Rules[0].InboundTag; len(got) != 2 || got[0] != TagTun || got[1] != "other" {
		t.Fatalf("inboundTag = %v", got)
	}
}

func TestBuildEnablesStatsAndKeepsPolicy(t *testing.T) {
	raw := json.RawMessage(`{"outbounds":[{"protocol":"freedom"}],"policy":{"levels":{"0":{"bufferSize":64}},"system":{"statsOutboundUplink":false}}}`)
	built, err := Build(raw, tunOptions)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Stats  *struct{} `json:"stats"`
		Policy struct {
			Levels map[string]struct {
				BufferSize int `json:"bufferSize"`
			} `json:"levels"`
			System map[string]bool `json:"system"`
		} `json:"policy"`
	}
	if err := json.Unmarshal(built, &out); err != nil {
		t.Fatal(err)
	}
	if out.Stats == nil || out.Policy.Levels["0"].BufferSize != 64 {
		t.Fatalf("policy = %s", built)
	}
	for _, k := range []string{"statsInboundUplink", "statsInboundDownlink", "statsOutboundUplink", "statsOutboundDownlink"} {
		if !out.Policy.System[k] {
			t.Fatalf("%s is off", k)
		}
	}
}

func TestBuildRejectsGarbage(t *testing.T) {
	for _, raw := range []string{"", "[]", "{}", `{"outbounds":null}`, "nope"} {
		if _, err := Build(json.RawMessage(raw), tunOptions); !errors.Is(err, ErrConfig) {
			t.Fatalf("%q: err = %v", raw, err)
		}
	}
}

func TestIPv6FollowsTheSubscriptionUnlessItIsSwitchedOff(t *testing.T) {
	tun := Options{Mode: ModeTun, Tun: Tun{Name: "spofy0", Resolver: []string{"172.19.0.2"}}}
	blocked := func(dns string, off bool) bool {
		t.Helper()
		o := tun
		o.Tweaks.NoIPv6 = off
		b, err := Build(json.RawMessage(`{"outbounds":[{"tag":"proxy","protocol":"vless"}],"dns":`+dns+`}`), o)
		if err != nil {
			t.Fatal(err)
		}
		var c struct {
			Routing struct {
				Rules []struct {
					IP  []string `json:"ip"`
					Out string   `json:"outboundTag"`
				} `json:"rules"`
			} `json:"routing"`
		}
		if err := json.Unmarshal(b, &c); err != nil {
			t.Fatal(err)
		}
		for _, r := range c.Routing.Rules {
			if len(r.IP) == 1 && r.IP[0] == "::/0" && r.Out == TagBlock {
				return true
			}
		}
		return false
	}
	for _, c := range []struct {
		dns  string
		off  bool
		want bool
	}{
		{`{"servers":["1.1.1.1"],"queryStrategy":"UseIPv4"}`, false, true},
		{`{"servers":["1.1.1.1"],"queryStrategy":"UseIP"}`, false, false},
		{`{"servers":["1.1.1.1"]}`, false, false},
		{`null`, false, false},
		{`{"servers":["1.1.1.1"]}`, true, true},
	} {
		if got := blocked(c.dns, c.off); got != c.want {
			t.Errorf("dns %s, switched off %v: IPv6 blocked %v, want %v", c.dns, c.off, got, c.want)
		}
	}
	probe, err := Build(json.RawMessage(`{"outbounds":[{"tag":"proxy","protocol":"vless"}],"dns":{"queryStrategy":"UseIPv4"}}`), Options{Mode: ModeProbe})
	if err != nil || strings.Contains(string(probe), "::/0") {
		t.Fatalf("a probe stays as the server gave it: %v %s", err, probe)
	}
}

func TestLocalAddressesAreNotSniffed(t *testing.T) {
	var s struct {
		Enabled bool     `json:"enabled"`
		IPs     []string `json:"ipsExcluded"`
	}
	if err := json.Unmarshal(local(json.RawMessage(`{"enabled":true,"ipsExcluded":["192.168.0.0/16","203.0.113.7/32"]}`)), &s); err != nil {
		t.Fatal(err)
	}
	if !s.Enabled || len(s.IPs) != len(nearby)+1 || s.IPs[1] != "203.0.113.7/32" {
		t.Fatalf("kept and added: %+v", s)
	}
	if got := string(local(json.RawMessage(`null`))); got != "null" {
		t.Fatalf("no sniffing stays none: %s", got)
	}
}

func TestWithoutIPv6TheNameHandlerAnswersAAAAEmptyNotRefused(t *testing.T) {
	tun := Options{Mode: ModeTun, Tun: Tun{Name: "spofy0", Resolver: []string{"172.19.0.2"}}}
	types := func(block string, off bool) string {
		t.Helper()
		o := tun
		o.Tweaks.NoIPv6 = off
		b, err := Build(json.RawMessage(`{"outbounds":[{"tag":"proxy","protocol":"vless"},{"tag":"dns-out","protocol":"dns","settings":{"nonIPQuery":"reject"`+block+`}}],"dns":{"servers":["1.1.1.1"]}}`), o)
		if err != nil {
			t.Fatal(err)
		}
		var c struct {
			Outbounds []struct {
				Protocol string                     `json:"protocol"`
				Settings map[string]json.RawMessage `json:"settings"`
			} `json:"outbounds"`
		}
		if err := json.Unmarshal(b, &c); err != nil {
			t.Fatal(err)
		}
		for _, ob := range c.Outbounds {
			if ob.Protocol == "dns" {
				if string(ob.Settings["nonIPQuery"]) != `"reject"` {
					t.Fatalf("the rest of the handler is kept: %s", ob.Settings)
				}
				return string(ob.Settings["blockTypes"])
			}
		}
		t.Fatal("no name handler")
		return ""
	}
	for _, c := range []struct {
		block string
		off   bool
		want  string
	}{
		{`,"blockTypes":[28]`, true, ""},
		{`,"blockTypes":[28,65]`, true, "[65]"},
		{`,"blockTypes":[28]`, false, "[28]"},
		{``, true, ""},
	} {
		if got := types(c.block, c.off); got != c.want {
			t.Errorf("%s, switched off %v: blockTypes %q, want %q", c.block, c.off, got, c.want)
		}
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	raw := servers(t, "../../testdata/sub/sub.json")[0]
	a, _ := Build(raw, tunOptions)
	b, _ := Build(raw, tunOptions)
	if !bytes.Equal(a, b) {
		t.Fatal("output differs between runs")
	}
}

func TestTweaksChangeOnlyWhatTheyName(t *testing.T) {
	server := json.RawMessage(`{"outbounds":[{"tag":"proxy","protocol":"vless","streamSettings":{"network":"tcp","security":"reality"}},{"tag":"h2","protocol":"vless","streamSettings":{"network":"hysteria","security":"tls"}},{"tag":"direct","protocol":"freedom"}],
		"dns":{"servers":["8.8.8.8",{"address":"77.88.8.8","domains":["domain:example.ru"]}],"hosts":{"a.example":"192.0.2.1"}},
		"routing":{"rules":[{"type":"field","ip":["geoip:ru"],"outboundTag":"direct"}]}}`)
	tun := Options{Mode: ModeTun, Tun: Tun{Name: "spofy0", Resolver: []string{"172.19.0.2"}}}
	plain, err := Build(server, tun)
	if err != nil {
		t.Fatal(err)
	}
	type shape struct {
		DNS struct {
			Servers  []any             `json:"servers"`
			Hosts    map[string]string `json:"hosts"`
			Strategy string            `json:"queryStrategy"`
		} `json:"dns"`
		Outbounds []struct {
			Tag      string `json:"tag"`
			Protocol string `json:"protocol"`
			Settings struct {
				Fragment *struct {
					Packets string `json:"packets"`
					Split   string `json:"maxSplit"`
				} `json:"fragment"`
			} `json:"settings"`
			Stream struct {
				Sockopt struct {
					Dialer string `json:"dialerProxy"`
				} `json:"sockopt"`
			} `json:"streamSettings"`
		} `json:"outbounds"`
		Routing struct {
			Rules []struct {
				In  []string `json:"inboundTag"`
				IP  []string `json:"ip"`
				Out string   `json:"outboundTag"`
			} `json:"rules"`
		} `json:"routing"`
	}
	read := func(o Options) shape {
		t.Helper()
		b, err := Build(server, o)
		if err != nil {
			t.Fatal(err)
		}
		var s shape
		if err := json.Unmarshal(b, &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if again, _ := Build(server, Options{Mode: ModeTun, Tun: tun.Tun, Tweaks: Tweaks{}}); !bytes.Equal(again, plain) {
		t.Fatal("empty tweaks changed the config")
	}
	o := tun
	o.Tweaks = Tweaks{Fragment: true}
	f := read(o)
	cut := false
	for _, out := range f.Outbounds {
		cut = cut || (out.Tag == TagFragment && out.Protocol == "freedom" && out.Settings.Fragment != nil && out.Settings.Fragment.Packets == "1-1" && out.Settings.Fragment.Split == "2-2")
	}
	if f.Outbounds[0].Stream.Sockopt.Dialer != TagFragment || f.Outbounds[1].Stream.Sockopt.Dialer != "" || !cut {
		t.Fatalf("fragment: %+v", f.Outbounds)
	}
	probe := read(Options{Mode: ModeProbe, Tweaks: Tweaks{Fragment: true, DNS: []string{"1.1.1.1"}, NoIPv6: true, LAN: true}})
	if probe.Outbounds[0].Stream.Sockopt.Dialer != TagFragment || len(probe.Routing.Rules) != 1 || len(probe.DNS.Servers) != 2 {
		t.Fatalf("a probe takes the fragment and nothing of the tunnel: %+v", probe)
	}
	o.Tweaks = Tweaks{DNS: []string{"1.1.1.1", "1.0.0.1"}, NoIPv6: true, LAN: true}
	d := read(o)
	if len(d.DNS.Servers) != 2 || d.DNS.Servers[0] != "1.1.1.1" || d.DNS.Hosts["a.example"] != "192.0.2.1" || d.DNS.Strategy != "UseIPv4" {
		t.Fatalf("dns: %+v", d.DNS)
	}
	r := d.Routing.Rules
	if len(r) != 6 || r[2].Out != "proxy" || r[2].IP[0] != "1.1.1.1" || r[3].IP[0] != "::/0" || r[3].Out != TagBlock || r[4].Out != TagLAN || r[4].In[0] != TagTun || r[5].IP[0] != "geoip:ru" {
		t.Fatalf("rules: %+v", r)
	}
	lan := false
	for _, o := range d.Outbounds {
		lan = lan || (o.Tag == TagLAN && o.Protocol == "freedom")
	}
	if !lan {
		t.Fatalf("no way out for the local network: %+v", d.Outbounds)
	}
}
