package xcfg

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

type Mode int

const (
	ModeTun Mode = iota
	ModeProbe
	ModeProxy
)

const (
	TagTun        = "tun"
	TagOutside    = "spofy-outside"
	TagOutsideDNS = "spofy-outside-dns"
	outsideDNS    = "8.8.8.8:53"
	Loopback      = "127.0.0.1"
)

type Tun struct {
	Name     string
	MTU      int
	Gateway  []string
	DNS      []string
	Routes   []string
	OutIface string
	WfpBlock []string
	Resolver []string
}

type Options struct {
	Mode     Mode
	Tun      Tun
	Port     int
	LogLevel string
	Tweaks   Tweaks
}

var ErrConfig = errors.New("xcfg: invalid server config")

var kept = []string{"dns", "fakedns", "routing", "outbounds", "policy", "observatory", "burstObservatory"}

var defaultSniffing = json.RawMessage(`{"enabled":true,"routeOnly":false,"destOverride":["http","tls","quic"]}`)

func Build(server json.RawMessage, o Options) ([]byte, error) {
	var in map[string]json.RawMessage
	if err := json.Unmarshal(server, &in); err != nil {
		return nil, ErrConfig
	}
	var outbounds []json.RawMessage
	if json.Unmarshal(in["outbounds"], &outbounds) != nil || len(outbounds) == 0 {
		return nil, ErrConfig
	}
	out := map[string]json.RawMessage{}
	for _, k := range kept {
		if v, ok := in[k]; ok && string(v) != "null" {
			out[k] = v
		}
	}
	if o.Mode == ModeProxy {
		delete(out, "fakedns")
		if v, ok := out["dns"]; ok {
			dns, err := withoutFake(v)
			if err != nil {
				return nil, err
			}
			out["dns"] = dns
		}
	}
	if o.Mode == ModeProbe {
		out["inbounds"] = json.RawMessage(`[]`)
		if err := tweak(out, o); err != nil {
			return nil, err
		}
	} else {
		sniffing, oldTags := inspect(in["inbounds"])
		inbound, tag, err := inboundFor(o, sniffing)
		if err != nil {
			return nil, err
		}
		out["inbounds"] = mustJSON([]any{inbound})
		if r, ok := out["routing"]; ok {
			fixed, err := retag(r, oldTags, tag)
			if err != nil {
				return nil, err
			}
			out["routing"] = fixed
		}
		if err := tweak(out, o); err != nil {
			return nil, err
		}
		if json.Unmarshal(out["outbounds"], &outbounds) != nil {
			return nil, ErrConfig
		}
		if o.Mode == ModeTun && len(o.Tun.Resolver) > 0 {
			outs, routing, err := resolver(outbounds, out["routing"], o.Tun.Resolver)
			if err != nil {
				return nil, err
			}
			out["outbounds"], out["routing"] = mustJSON(outs), routing
		}
	}
	level := o.LogLevel
	if level == "" {
		level = "warning"
	}
	out["log"] = mustJSON(map[string]string{"loglevel": level, "access": "none"})
	out["stats"] = json.RawMessage(`{}`)
	policy, err := withStats(out["policy"])
	if err != nil {
		return nil, err
	}
	out["policy"] = policy
	return json.Marshal(out)
}

func Local(cfg []byte) bool {
	var c struct {
		Inbounds []struct {
			Protocol string `json:"protocol"`
		} `json:"inbounds"`
	}
	return json.Unmarshal(cfg, &c) == nil && len(c.Inbounds) == 1 && c.Inbounds[0].Protocol == "mixed"
}

func MainTag(server json.RawMessage) string {
	var in struct {
		Outbounds []struct {
			Tag string `json:"tag"`
		} `json:"outbounds"`
	}
	if json.Unmarshal(server, &in) != nil || len(in.Outbounds) == 0 {
		return ""
	}
	return in.Outbounds[0].Tag
}

func inspect(raw json.RawMessage) (json.RawMessage, []string) {
	var inbounds []struct {
		Tag      string          `json:"tag"`
		Sniffing json.RawMessage `json:"sniffing"`
	}
	sniffing := defaultSniffing
	if json.Unmarshal(raw, &inbounds) != nil {
		return sniffing, nil
	}
	var tags []string
	found := false
	for _, in := range inbounds {
		if in.Tag != "" {
			tags = append(tags, in.Tag)
		}
		if !found && len(in.Sniffing) > 0 && string(in.Sniffing) != "null" {
			sniffing, found = in.Sniffing, true
		}
	}
	return sniffing, tags
}

func inboundFor(o Options, sniffing json.RawMessage) (map[string]any, string, error) {
	switch o.Mode {
	case ModeTun:
		t := o.Tun
		settings := map[string]any{}
		put := func(k string, v []string) {
			if len(v) > 0 {
				settings[k] = v
			}
		}
		if t.Name != "" {
			settings["name"] = t.Name
		}
		if t.MTU > 0 {
			settings["mtu"] = t.MTU
		}
		put("gateway", t.Gateway)
		put("dns", t.DNS)
		put("autoSystemRoutingTable", t.Routes)
		put("autoSystemWfpBlockLeak", t.WfpBlock)
		if t.OutIface != "" {
			settings["autoOutboundsInterface"] = t.OutIface
		}
		return map[string]any{"tag": TagTun, "protocol": "tun", "port": 0, "settings": settings, "sniffing": local(sniffing)}, TagTun, nil
	case ModeProxy:
		if o.Port <= 0 || o.Port > 65535 {
			return nil, "", ErrConfig
		}
		settings := map[string]any{"auth": "noauth", "udp": true, "ip": Loopback}
		return map[string]any{"tag": TagTun, "protocol": "mixed", "listen": Loopback, "port": o.Port, "settings": settings, "sniffing": local(unfaked(sniffing))}, TagTun, nil
	}
	return nil, "", ErrConfig
}

func fake(v json.RawMessage) bool {
	var name string
	if json.Unmarshal(v, &name) != nil {
		var server struct {
			Address string `json:"address"`
		}
		json.Unmarshal(v, &server)
		name = server.Address
	}
	return strings.EqualFold(name, "fakedns")
}

func withoutFake(raw json.RawMessage) (json.RawMessage, error) {
	dns := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &dns); err != nil {
		return nil, ErrConfig
	}
	var servers []json.RawMessage
	if len(dns["servers"]) == 0 || string(dns["servers"]) == "null" {
		return raw, nil
	}
	if err := json.Unmarshal(dns["servers"], &servers); err != nil {
		return nil, ErrConfig
	}
	kept := slices.DeleteFunc(slices.Clone(servers), fake)
	if len(kept) == len(servers) {
		return raw, nil
	}
	dns["servers"] = mustJSON(kept)
	return json.Marshal(dns)
}

func unfaked(sniffing json.RawMessage) json.RawMessage {
	var s map[string]json.RawMessage
	var over []string
	if json.Unmarshal(sniffing, &s) != nil || s == nil || json.Unmarshal(s["destOverride"], &over) != nil {
		return sniffing
	}
	kept := slices.DeleteFunc(slices.Clone(over), func(d string) bool { return strings.HasPrefix(d, "fakedns") })
	if len(kept) == len(over) {
		return sniffing
	}
	s["destOverride"] = mustJSON(kept)
	return mustJSON(s)
}

func resolver(outbounds []json.RawMessage, raw json.RawMessage, addrs []string) ([]json.RawMessage, json.RawMessage, error) {
	find := func(protocol, fallback string) string {
		for _, o := range outbounds {
			var shape struct {
				Tag      string `json:"tag"`
				Protocol string `json:"protocol"`
			}
			if json.Unmarshal(o, &shape) == nil && shape.Protocol == protocol && shape.Tag != "" {
				return shape.Tag
			}
		}
		outbounds = append(outbounds, mustJSON(map[string]string{"tag": fallback, "protocol": protocol}))
		return fallback
	}
	dns, drop := find("dns", "spofy-dns"), find("blackhole", "spofy-drop")
	outbounds = append(outbounds,
		mustJSON(map[string]string{"tag": TagOutside, "protocol": "freedom"}),
		mustJSON(map[string]any{"tag": TagOutsideDNS, "protocol": "freedom", "settings": map[string]string{"redirect": outsideDNS}}))
	routing := map[string]json.RawMessage{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &routing); err != nil {
			return nil, nil, ErrConfig
		}
	}
	var rules []json.RawMessage
	if len(routing["rules"]) > 0 && string(routing["rules"]) != "null" {
		if err := json.Unmarshal(routing["rules"], &rules); err != nil {
			return nil, nil, ErrConfig
		}
	}
	own := []json.RawMessage{
		mustJSON(map[string]any{"type": "field", "inboundTag": []string{TagTun}, "ip": addrs, "port": "53", "outboundTag": dns}),
		mustJSON(map[string]any{"type": "field", "inboundTag": []string{TagTun}, "ip": addrs, "outboundTag": drop}),
	}
	routing["rules"] = mustJSON(append(own, rules...))
	fixed, err := json.Marshal(routing)
	return outbounds, fixed, err
}

func retag(raw json.RawMessage, old []string, tag string) (json.RawMessage, error) {
	var routing map[string]json.RawMessage
	if err := json.Unmarshal(raw, &routing); err != nil {
		return nil, ErrConfig
	}
	var rules []map[string]json.RawMessage
	if len(routing["rules"]) == 0 || string(routing["rules"]) == "null" {
		return raw, nil
	}
	if err := json.Unmarshal(routing["rules"], &rules); err != nil {
		return nil, ErrConfig
	}
	changed := false
	for _, rule := range rules {
		v, ok := rule["inboundTag"]
		if !ok {
			continue
		}
		var tags []string
		if err := json.Unmarshal(v, &tags); err != nil {
			return nil, ErrConfig
		}
		var next []string
		for _, t := range tags {
			if slices.Contains(old, t) {
				t = tag
			}
			if !slices.Contains(next, t) {
				next = append(next, t)
			}
		}
		rule["inboundTag"] = mustJSON(next)
		changed = true
	}
	if !changed {
		return raw, nil
	}
	routing["rules"] = mustJSON(rules)
	return json.Marshal(routing)
}

func withStats(raw json.RawMessage) (json.RawMessage, error) {
	policy := map[string]json.RawMessage{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &policy); err != nil {
			return nil, ErrConfig
		}
	}
	system := map[string]json.RawMessage{}
	if len(policy["system"]) > 0 {
		if err := json.Unmarshal(policy["system"], &system); err != nil {
			return nil, ErrConfig
		}
	}
	for _, k := range []string{"statsInboundUplink", "statsInboundDownlink", "statsOutboundUplink", "statsOutboundDownlink"} {
		system[k] = json.RawMessage(`true`)
	}
	policy["system"] = mustJSON(system)
	return json.Marshal(policy)
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

var nearby = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "100.64.0.0/10", "fc00::/7", "fe80::/10"}

func local(sniffing json.RawMessage) json.RawMessage {
	var s map[string]json.RawMessage
	if json.Unmarshal(sniffing, &s) != nil || s == nil {
		return sniffing
	}
	var kept []string
	json.Unmarshal(s["ipsExcluded"], &kept)
	for _, r := range nearby {
		if !slices.Contains(kept, r) {
			kept = append(kept, r)
		}
	}
	s["ipsExcluded"] = mustJSON(kept)
	return mustJSON(s)
}
