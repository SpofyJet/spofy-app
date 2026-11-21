package xcfg

import (
	"cmp"
	"encoding/json"
	"net/netip"
	"slices"
	"strconv"
	"strings"
)

var strategies = []string{"asis", "useip", "useipv4", "useipv6", "useipv4v6", "useipv6v4", "forceip", "forceipv4", "forceipv6", "forceipv4v6", "forceipv6v4"}

func modernize(out map[string]json.RawMessage) error {
	var outbounds []map[string]json.RawMessage
	if err := json.Unmarshal(out["outbounds"], &outbounds); err != nil {
		return ErrConfig
	}
	changed := false
	for _, ob := range outbounds {
		var protocol string
		json.Unmarshal(ob["protocol"], &protocol)
		switch protocol {
		case "dns":
			changed = dnsRules(ob) || changed
		case "freedom":
			changed = sockStrategy(ob) || changed
		}
	}
	if changed {
		out["outbounds"] = mustJSON(outbounds)
	}
	return nil
}

func given(m map[string]json.RawMessage, k string) bool {
	v, ok := m[k]
	return ok && string(v) != "null"
}

func dnsRules(ob map[string]json.RawMessage) bool {
	var s map[string]json.RawMessage
	if json.Unmarshal(ob["settings"], &s) != nil || (!given(s, "nonIPQuery") && !given(s, "blockTypes")) || given(s, "rules") {
		return false
	}
	mode := "reject"
	if given(s, "nonIPQuery") {
		var m string
		if json.Unmarshal(s["nonIPQuery"], &m) != nil {
			return false
		}
		mode = cmp.Or(m, mode)
	}
	if !slices.Contains([]string{"reject", "drop", "skip"}, mode) {
		return false
	}
	var types []int
	if given(s, "blockTypes") && json.Unmarshal(s["blockTypes"], &types) != nil {
		return false
	}
	var rules []map[string]any
	if len(types) > 0 {
		list := make([]string, len(types))
		for i, t := range types {
			if t < 0 || t > 65535 {
				return false
			}
			list[i] = strconv.Itoa(t)
		}
		r := map[string]any{"action": "drop", "qType": strings.Join(list, ",")}
		if mode == "reject" {
			r["action"], r["rCode"] = "return", 5
		}
		rules = append(rules, r)
	}
	rest := map[string]any{"action": map[string]string{"reject": "return", "drop": "drop", "skip": "direct"}[mode]}
	if mode == "reject" {
		rest["rCode"] = 5
	}
	rules = append(rules, map[string]any{"action": "hijack", "qType": "1,28"}, rest)
	delete(s, "nonIPQuery")
	delete(s, "blockTypes")
	s["rules"] = mustJSON(rules)
	ob["settings"] = mustJSON(s)
	return true
}

func sockStrategy(ob map[string]json.RawMessage) bool {
	var settings map[string]json.RawMessage
	if given(ob, "settings") && json.Unmarshal(ob["settings"], &settings) != nil {
		return false
	}
	var outer, target, domain string
	for _, f := range []struct {
		m map[string]json.RawMessage
		k string
		v *string
	}{{ob, "targetStrategy", &outer}, {settings, "targetStrategy", &target}, {settings, "domainStrategy", &domain}} {
		if given(f.m, f.k) && (json.Unmarshal(f.m[f.k], f.v) != nil || !slices.Contains(strategies, strings.ToLower(cmp.Or(*f.v, "asis")))) {
			return false
		}
	}
	strategy := cmp.Or(target, domain)
	if !strings.EqualFold(cmp.Or(outer, "asis"), "asis") {
		strategy = outer
	}
	if strings.EqualFold(cmp.Or(strategy, "asis"), "asis") {
		return false
	}
	stream := map[string]json.RawMessage{}
	sock := map[string]json.RawMessage{}
	if given(ob, "streamSettings") && json.Unmarshal(ob["streamSettings"], &stream) != nil || given(stream, "sockopt") && json.Unmarshal(stream["sockopt"], &sock) != nil {
		return false
	}
	sock["domainStrategy"] = mustJSON(strategy)
	stream["sockopt"] = mustJSON(sock)
	ob["streamSettings"] = mustJSON(stream)
	delete(ob, "targetStrategy")
	if settings != nil {
		delete(settings, "targetStrategy")
		delete(settings, "domainStrategy")
		ob["settings"] = mustJSON(settings)
	}
	return true
}

func realBehindFake(out map[string]json.RawMessage) {
	var dns map[string]json.RawMessage
	var servers []json.RawMessage
	if json.Unmarshal(out["dns"], &dns) != nil || json.Unmarshal(dns["servers"], &servers) != nil || string(dns["enableParallelQuery"]) == "true" {
		return
	}
	v4, v6 := asks(dns["queryStrategy"])
	has4, has6, ok := pools(out["fakedns"], v4, v6)
	if !ok {
		return
	}
	changed := false
	for i, raw := range servers {
		var s map[string]json.RawMessage
		var address string
		if json.Unmarshal(raw, &s) != nil || json.Unmarshal(s["address"], &address) != nil || !strings.EqualFold(address, "fakedns") || string(s["finalQuery"]) != "true" {
			continue
		}
		w4, w6 := asks(s["queryStrategy"])
		if (v4 && w4 && !has4) || (v6 && w6 && !has6) {
			continue
		}
		delete(s, "finalQuery")
		servers[i] = mustJSON(s)
		changed = true
	}
	if changed {
		dns["servers"] = mustJSON(servers)
		out["dns"] = mustJSON(dns)
	}
}

func asks(raw json.RawMessage) (v4, v6 bool) {
	var s string
	json.Unmarshal(raw, &s)
	switch strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(s)) {
	case "useip4", "useipv4":
		return true, false
	case "useip6", "useipv6":
		return false, true
	}
	return true, true
}

func pools(raw json.RawMessage, v4, v6 bool) (has4, has6, ok bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return v4, v6, true
	}
	type pool struct {
		IPPool string `json:"ipPool"`
	}
	var list []pool
	if json.Unmarshal(raw, &list) != nil {
		var one pool
		if json.Unmarshal(raw, &one) != nil {
			return false, false, false
		}
		list = []pool{one}
	}
	for _, p := range list {
		prefix, err := netip.ParsePrefix(p.IPPool)
		if err != nil {
			return false, false, false
		}
		four := prefix.Addr().Unmap().Is4()
		has4, has6 = has4 || four, has6 || !four
	}
	return has4, has6, true
}
