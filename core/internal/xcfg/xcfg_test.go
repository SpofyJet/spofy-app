package xcfg

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
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
			if !bytes.Equal(in[k], out[k]) && !sameButRewritten(in[k], out[k]) {
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

func sameButRewritten(in, out json.RawMessage) bool {
	var a, b any
	if json.Unmarshal(in, &a) != nil || json.Unmarshal(out, &b) != nil {
		return false
	}
	return reflect.DeepEqual(rewritable(a), rewritable(b))
}

func rewritable(v any) any {
	if m, ok := v.(map[string]any); ok {
		for _, k := range []string{"rules", "servers"} {
			if m[k] != nil {
				m[k] = rewritable(m[k])
			}
		}
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
		if ok && m["address"] == "fakedns" {
			delete(m, "finalQuery")
		}
		if s, ok := m["settings"].(map[string]any); ok && m["protocol"] == "dns" {
			delete(s, "blockTypes")
			delete(s, "nonIPQuery")
			delete(s, "rules")
		}
		if s, ok := m["settings"].(map[string]any); ok && m["protocol"] == "freedom" {
			delete(s, "domainStrategy")
			delete(m, "streamSettings")
		}
		kept = append(kept, e)
	}
	return kept
}

func TestTheSubscriptionIsWrittenInTheCurrentSyntax(t *testing.T) {
	raw := servers(t, "../../testdata/sub/sub.json")[0]
	built, err := Build(raw, tunOptions)
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		DNS struct {
			Servers []json.RawMessage `json:"servers"`
		} `json:"dns"`
		Outbounds []struct {
			Tag      string          `json:"tag"`
			Settings json.RawMessage `json:"settings"`
			Stream   json.RawMessage `json:"streamSettings"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal(built, &c); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, o := range c.Outbounds {
		got[o.Tag] = string(o.Settings) + " " + string(o.Stream)
	}
	if got["dns-out"] != `{"rules":[{"action":"hijack","qType":"1,28"},{"action":"return","rCode":5}]} ` {
		t.Errorf("name handler: %s", got["dns-out"])
	}
	if got["direct"] != `{} {"sockopt":{"domainStrategy":"UseIPv4"}}` {
		t.Errorf("direct: %s", got["direct"])
	}
	if last := string(c.DNS.Servers[len(c.DNS.Servers)-1]); strings.Contains(last, "finalQuery") || !strings.Contains(last, "fakedns") {
		t.Errorf("fake names: %s", last)
	}
}

func TestOldFieldsKeepTheirMeaningInTheNewForm(t *testing.T) {
	build := func(outbound string) map[string]json.RawMessage {
		t.Helper()
		b, err := Build(json.RawMessage(`{"outbounds":[{"tag":"proxy","protocol":"vless"},`+outbound+`]}`), Options{Mode: ModeProbe})
		if err != nil {
			t.Fatal(err)
		}
		var c struct {
			Outbounds []map[string]json.RawMessage `json:"outbounds"`
		}
		if err := json.Unmarshal(b, &c); err != nil {
			t.Fatal(err)
		}
		return c.Outbounds[1]
	}
	for _, c := range []struct{ settings, rules string }{
		{`{"nonIPQuery":"reject","blockTypes":[28]}`, `[{"action":"return","qType":"28","rCode":5},{"action":"hijack","qType":"1,28"},{"action":"return","rCode":5}]`},
		{`{"nonIPQuery":"drop","blockTypes":[28,65]}`, `[{"action":"drop","qType":"28,65"},{"action":"hijack","qType":"1,28"},{"action":"drop"}]`},
		{`{"nonIPQuery":"skip","blockTypes":[65]}`, `[{"action":"drop","qType":"65"},{"action":"hijack","qType":"1,28"},{"action":"direct"}]`},
		{`{"blockTypes":[]}`, `[{"action":"hijack","qType":"1,28"},{"action":"return","rCode":5}]`},
		{`{"nonIPQuery":"","port":53}`, `[{"action":"hijack","qType":"1,28"},{"action":"return","rCode":5}]`},
	} {
		ob := build(`{"tag":"d","protocol":"dns","settings":` + c.settings + `}`)
		var s map[string]json.RawMessage
		json.Unmarshal(ob["settings"], &s)
		if string(s["rules"]) != c.rules || s["nonIPQuery"] != nil || s["blockTypes"] != nil {
			t.Errorf("%s: %s", c.settings, ob["settings"])
		}
		if strings.Contains(c.settings, "port") && string(s["port"]) != "53" {
			t.Errorf("the rest of the handler is kept: %s", ob["settings"])
		}
	}
	for _, settings := range []string{`{"nonIPQuery":"refuse"}`, `{"blockTypes":[70000]}`, `{"blockTypes":[28],"rules":[]}`, `{"rules":[{"action":"hijack"}]}`, `{}`} {
		ob := build(`{"tag":"d","protocol":"dns","settings":` + settings + `}`)
		if string(ob["settings"]) != settings {
			t.Errorf("what the core would refuse or already reads stays as it was: %s -> %s", settings, ob["settings"])
		}
	}
	for _, c := range []struct{ outbound, settings, stream string }{
		{`{"tag":"f","protocol":"freedom","settings":{"domainStrategy":"UseIPv4"}}`, `{}`, `{"sockopt":{"domainStrategy":"UseIPv4"}}`},
		{`{"tag":"f","protocol":"freedom","settings":{"targetStrategy":"ForceIP","domainStrategy":"UseIPv4","redirect":"1.1.1.1:53"}}`, `{"redirect":"1.1.1.1:53"}`, `{"sockopt":{"domainStrategy":"ForceIP"}}`},
		{`{"tag":"f","protocol":"freedom","targetStrategy":"UseIPv6","settings":{"domainStrategy":"UseIPv4"},"streamSettings":{"sockopt":{"mark":7,"domainStrategy":"UseIP"}}}`, `{}`, `{"sockopt":{"domainStrategy":"UseIPv6","mark":7}}`},
		{`{"tag":"f","protocol":"freedom","targetStrategy":"UseIPv4"}`, ``, `{"sockopt":{"domainStrategy":"UseIPv4"}}`},
	} {
		ob := build(c.outbound)
		if string(ob["settings"]) != c.settings || string(ob["streamSettings"]) != c.stream || ob["targetStrategy"] != nil {
			t.Errorf("%s: settings %s, stream %s", c.outbound, ob["settings"], ob["streamSettings"])
		}
	}
	for _, outbound := range []string{
		`{"tag":"f","protocol":"freedom","settings":{"domainStrategy":"AsIs"}}`,
		`{"tag":"f","protocol":"freedom","settings":{"domainStrategy":"Sometimes"}}`,
		`{"tag":"f","protocol":"freedom"}`,
		`{"tag":"v","protocol":"vless","targetStrategy":"UseIPv4"}`,
	} {
		var want, got any
		json.Unmarshal([]byte(outbound), &want)
		raw, _ := json.Marshal(build(outbound))
		json.Unmarshal(raw, &got)
		if !reflect.DeepEqual(want, got) {
			t.Errorf("left as it was: %s -> %s", outbound, raw)
		}
	}
}

func TestFakeNamesStayForAppsWhileTheCoreFindsRealOnes(t *testing.T) {
	fake := `{"address":"fakedns","domains":["regexp:.*"],"finalQuery":true,"skipFallback":true}`
	final := func(dns, pools string) bool {
		t.Helper()
		server := `{"outbounds":[{"tag":"proxy","protocol":"vless"}],"dns":` + dns
		if pools != "" {
			server += `,"fakedns":` + pools
		}
		b, err := Build(json.RawMessage(server+`}`), Options{Mode: ModeTun, Tun: Tun{Name: "spofy0", Resolver: []string{"172.19.0.2"}}})
		if err != nil {
			t.Fatal(err)
		}
		return strings.Contains(string(b), `"finalQuery":true`)
	}
	four, both := `[{"ipPool":"198.18.0.0/15","poolSize":65535}]`, `[{"ipPool":"198.18.0.0/15","poolSize":100},{"ipPool":"fc00::/18","poolSize":100}]`
	for _, c := range []struct {
		dns, pools string
		final      bool
	}{
		{`{"servers":["https://8.8.8.8/dns-query",` + fake + `],"queryStrategy":"UseIPv4"}`, four, false},
		{`{"servers":["https://8.8.8.8/dns-query",` + fake + `],"queryStrategy":"UseIPv4"}`, `{"ipPool":"198.18.0.0/15","poolSize":65535}`, false},
		{`{"servers":["https://8.8.8.8/dns-query",` + fake + `]}`, both, false},
		{`{"servers":["https://8.8.8.8/dns-query",` + fake + `]}`, ``, false},
		{`{"servers":["https://8.8.8.8/dns-query",` + strings.Replace(fake, `"finalQuery"`, `"queryStrategy":"UseIPv4","finalQuery"`, 1) + `]}`, four, false},
		{`{"servers":["https://8.8.8.8/dns-query",` + fake + `]}`, four, true},
		{`{"servers":["https://8.8.8.8/dns-query",` + fake + `],"queryStrategy":"UseIPv4","enableParallelQuery":true}`, four, true},
		{`{"servers":[{"address":"https://8.8.8.8/dns-query","finalQuery":true},` + fake + `],"queryStrategy":"UseIPv4"}`, four, true},
		{`{"servers":["https://8.8.8.8/dns-query",` + fake + `],"queryStrategy":"UseIPv4"}`, `[{"ipPool":"not a pool"}]`, true},
	} {
		if got := final(c.dns, c.pools); got != c.final {
			t.Errorf("%s with %s: final %v, want %v", c.dns, c.pools, got, c.final)
		}
	}
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
		for _, o := range []Options{tunOptions, {Mode: ModeProbe}, proxyOptions} {
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
					Settings struct {
						Auth string `json:"auth"`
						IP   string `json:"ip"`
					} `json:"settings"`
				} `json:"inbounds"`
			}
			if err := json.Unmarshal(built, &out); err != nil {
				t.Fatal(err)
			}
			if out.API != nil || out.Metrics != nil {
				t.Fatalf("#%d mode %d: a control port is kept: %s", i, o.Mode, built)
			}
			for _, in := range out.Inbounds {
				tunnel := in.Protocol == "tun" && in.Listen == nil && (in.Port == nil || string(in.Port) == "0")
				local := o.Mode == ModeProxy && in.Protocol == "mixed" && string(in.Listen) == `"127.0.0.1"` && string(in.Port) == "41234" && in.Settings.IP == Loopback
				if !tunnel && !local {
					t.Fatalf("#%d mode %d: a way in that other programs can reach: %s", i, o.Mode, built)
				}
			}
			if want := map[Mode]int{ModeTun: 1, ModeProbe: 0, ModeProxy: 1}[o.Mode]; len(out.Inbounds) != want {
				t.Fatalf("#%d mode %d: %d ways in", i, o.Mode, len(out.Inbounds))
			}
		}
	}
}

var proxyOptions = Options{Mode: ModeProxy, Port: 41234}

func TestBuildProxyKeepsServerLogicAndListensOnlyOnTheLoopback(t *testing.T) {
	faked := 0
	for i, raw := range servers(t, "../../testdata/sub/sub.json") {
		built, err := Build(raw, proxyOptions)
		if err != nil {
			t.Fatalf("server %d: %v", i, err)
		}
		var in, out map[string]json.RawMessage
		json.Unmarshal(raw, &in)
		if err := json.Unmarshal(built, &out); err != nil {
			t.Fatalf("server %d: %v", i, err)
		}
		for _, k := range []string{"outbounds", "routing", "burstObservatory"} {
			if !bytes.Equal(in[k], out[k]) && !sameButRewritten(in[k], out[k]) {
				t.Fatalf("server %d: section %s changed", i, k)
			}
		}
		if _, ok := in["fakedns"]; ok {
			faked++
		}
		if _, ok := out["fakedns"]; ok || bytes.Contains(built, []byte("fakedns")) {
			t.Fatalf("server %d: fake addresses are kept where no program asks the core for names: %s", i, built)
		}
		var was, now struct {
			Servers []json.RawMessage `json:"servers"`
			Rest    map[string]any    `json:"hosts"`
			Order   string            `json:"queryStrategy"`
		}
		json.Unmarshal(in["dns"], &was)
		json.Unmarshal(out["dns"], &now)
		kept := slices.DeleteFunc(slices.Clone(was.Servers), fake)
		if len(now.Servers) != len(kept) || !reflect.DeepEqual(was.Rest, now.Rest) || was.Order != now.Order {
			t.Fatalf("server %d: dns = %s", i, out["dns"])
		}
		for n := range kept {
			if !bytes.Equal(kept[n], now.Servers[n]) {
				t.Fatalf("server %d: dns server %d = %s", i, n, now.Servers[n])
			}
		}
		var inbounds []struct {
			Tag      string `json:"tag"`
			Protocol string `json:"protocol"`
			Listen   string `json:"listen"`
			Port     int    `json:"port"`
			Settings struct {
				Auth string `json:"auth"`
				UDP  bool   `json:"udp"`
				IP   string `json:"ip"`
			} `json:"settings"`
			Sniffing struct {
				Enabled  bool     `json:"enabled"`
				Over     []string `json:"destOverride"`
				Excluded []string `json:"ipsExcluded"`
			} `json:"sniffing"`
		}
		json.Unmarshal(out["inbounds"], &inbounds)
		if len(inbounds) != 1 || inbounds[0].Tag != TagTun || inbounds[0].Protocol != "mixed" || inbounds[0].Listen != Loopback || inbounds[0].Port != 41234 ||
			inbounds[0].Settings.Auth != "noauth" || !inbounds[0].Settings.UDP || inbounds[0].Settings.IP != Loopback {
			t.Fatalf("server %d: inbounds = %s", i, out["inbounds"])
		}
		if s := inbounds[0].Sniffing; !s.Enabled || !reflect.DeepEqual(s.Over, []string{"http", "tls", "quic"}) || len(s.Excluded) != len(nearby) {
			t.Fatalf("server %d: sniffing = %+v", i, s)
		}
	}
	if faked == 0 {
		t.Fatal("the sample has no server with fake addresses: the test checks nothing")
	}
	for _, port := range []int{0, -1, 65536} {
		if _, err := Build(servers(t, "../../testdata/sub/sub.json")[0], Options{Mode: ModeProxy, Port: port}); !errors.Is(err, ErrConfig) {
			t.Fatalf("port %d: %v", port, err)
		}
	}
}

func TestInProxyModeOnlyTheFragmentApplies(t *testing.T) {
	server := json.RawMessage(`{"outbounds":[{"tag":"proxy","protocol":"vless","streamSettings":{"network":"tcp","security":"reality"}},{"tag":"dns-out","protocol":"dns","settings":{"blockTypes":[28]}}],
		"dns":{"servers":["8.8.8.8",{"address":"fakedns","domains":["domain:x.example"]}],"queryStrategy":"UseIPv4"},
		"inbounds":[{"tag":"socks","protocol":"socks","listen":"0.0.0.0","port":1080}],
		"routing":{"rules":[{"type":"field","inboundTag":["socks"],"port":"53","outboundTag":"dns-out"}]}}`)
	o := proxyOptions
	o.Tweaks = Tweaks{Fragment: true, DNS: []string{"1.1.1.1"}, NoIPv6: true, LAN: true}
	built, err := Build(server, o)
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		DNS struct {
			Servers  []any  `json:"servers"`
			Strategy string `json:"queryStrategy"`
		} `json:"dns"`
		Outbounds []struct {
			Tag      string                     `json:"tag"`
			Settings map[string]json.RawMessage `json:"settings"`
			Stream   struct {
				Sockopt struct {
					Dialer string `json:"dialerProxy"`
				} `json:"sockopt"`
			} `json:"streamSettings"`
		} `json:"outbounds"`
		Routing struct {
			Rules []struct {
				In  []string `json:"inboundTag"`
				Out string   `json:"outboundTag"`
			} `json:"rules"`
		} `json:"routing"`
	}
	if err := json.Unmarshal(built, &c); err != nil {
		t.Fatal(err)
	}
	if c.Outbounds[0].Stream.Sockopt.Dialer != TagFragment || len(c.Outbounds) != 3 || c.Outbounds[2].Tag != TagFragment {
		t.Fatalf("the fragment is the outbound's business and applies: %+v", c.Outbounds)
	}
	if len(c.DNS.Servers) != 1 || c.DNS.Servers[0] != "8.8.8.8" || c.DNS.Strategy != "UseIPv4" || string(c.Outbounds[1].Settings["rules"]) != `[{"action":"return","qType":"28","rCode":5},{"action":"hijack","qType":"1,28"},{"action":"return","rCode":5}]` {
		t.Fatalf("programs ask the system for names, the subscription keeps its own: %+v", c.DNS)
	}
	if r := c.Routing.Rules; len(r) != 1 || r[0].In[0] != TagTun || r[0].Out != "dns-out" {
		t.Fatalf("no rules of the tunnel: %+v", r)
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
	rules := func(settings string, off bool) string {
		t.Helper()
		o := tun
		o.Tweaks.NoIPv6 = off
		b, err := Build(json.RawMessage(`{"outbounds":[{"tag":"proxy","protocol":"vless"},{"tag":"dns-out","protocol":"dns","settings":`+settings+`}],"dns":{"servers":["1.1.1.1"]}}`), o)
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
				return string(ob.Settings["rules"])
			}
		}
		t.Fatal("no name handler")
		return ""
	}
	tail := `{"action":"hijack","qType":"1,28"},{"action":"return","rCode":5}]`
	for _, c := range []struct {
		settings string
		off      bool
		want     string
	}{
		{`{"nonIPQuery":"reject","blockTypes":[28]}`, true, `[` + tail},
		{`{"nonIPQuery":"reject","blockTypes":[28,65]}`, true, `[{"action":"return","qType":"65","rCode":5},` + tail},
		{`{"nonIPQuery":"reject","blockTypes":[28]}`, false, `[{"action":"return","qType":"28","rCode":5},` + tail},
		{`{"nonIPQuery":"reject"}`, true, `[` + tail},
		{`{"rules":[{"action":"drop","qType":"1-100"},{"action":"return","qType":28},{"action":"hijack","qType":"28"},{"action":"return"}]}`, true,
			`[{"action":"drop","qType":"1-27,29-100"},{"action":"hijack","qType":"28"},{"action":"return"}]`},
		{`{"rules":[{"action":"return","qType":"env:TYPES"}]}`, true, `[{"action":"return","qType":"env:TYPES"}]`},
	} {
		if got := rules(c.settings, c.off); got != c.want {
			t.Errorf("%s, switched off %v:\n got %s\nwant %s", c.settings, c.off, got, c.want)
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
			Stream   struct {
				Sockopt struct {
					Dialer string `json:"dialerProxy"`
				} `json:"sockopt"`
				Mask struct {
					TCP []struct {
						Type     string `json:"type"`
						Settings struct {
							Packets string `json:"packets"`
							Length  string `json:"length"`
							Delay   string `json:"delay"`
							Split   string `json:"maxSplit"`
						} `json:"settings"`
					} `json:"tcp"`
				} `json:"finalmask"`
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
		if m := out.Stream.Mask.TCP; out.Tag == TagFragment && out.Protocol == "freedom" && len(m) == 1 && m[0].Type == "fragment" {
			s := m[0].Settings
			cut = s.Packets == "1-1" && s.Length == "128-137" && s.Delay == "0-0" && s.Split == "2-2"
		}
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
