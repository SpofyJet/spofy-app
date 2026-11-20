package xcfg

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
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
			if !bytes.Equal(in[k], out[k]) {
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
		if !bytes.Equal(orig[0].Sniffing, inbounds[0].Sniffing) {
			t.Fatalf("server %d: sniffing changed", i)
		}
		if string(out["log"]) != `{"access":"none","loglevel":"warning"}` {
			t.Fatalf("server %d: log = %s", i, out["log"])
		}
	}
}

func TestBuildTunGivesTheResolverAddressToTheNameHandlerAndRefusesTheRest(t *testing.T) {
	type shape struct {
		Outbounds []struct {
			Tag      string `json:"tag"`
			Protocol string `json:"protocol"`
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
	if n := len(bare.Outbounds); n != 3 || bare.Outbounds[0].Tag != "proxy" || bare.Outbounds[1].Protocol != "dns" || bare.Outbounds[2].Protocol != "blackhole" {
		t.Fatalf("outbounds = %+v", bare.Outbounds)
	}
	if r := bare.Routing.Rules; len(r) != 2 || r[0].Port != "53" || r[0].Out != bare.Outbounds[1].Tag || r[0].IP[0] != "172.19.0.2" || r[0].In[0] != TagTun ||
		r[1].Port != "" || r[1].Out != bare.Outbounds[2].Tag || r[1].IP[0] != "172.19.0.2" {
		t.Fatalf("rules = %+v", r)
	}
	full := build(`{"outbounds":[{"tag":"proxy","protocol":"vless"},{"tag":"names","protocol":"dns"},{"tag":"none","protocol":"blackhole"}],
		"routing":{"domainStrategy":"IPIfNonMatch","rules":[{"type":"field","port":"53","outboundTag":"names"}]}}`)
	if n := len(full.Outbounds); n != 3 {
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

func TestBuildIsDeterministic(t *testing.T) {
	raw := servers(t, "../../testdata/sub/sub.json")[0]
	a, _ := Build(raw, tunOptions)
	b, _ := Build(raw, tunOptions)
	if !bytes.Equal(a, b) {
		t.Fatal("output differs between runs")
	}
}
