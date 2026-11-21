package xcfg

import (
	"encoding/json"
	"slices"
)

type Tweaks struct {
	Fragment bool
	DNS      []string
	NoIPv6   bool
	LAN      bool
}

const (
	TagFragment = "spofy-fragment"
	TagLAN      = "spofy-lan"
	TagBlock    = "spofy-block"
)

var (
	fragment = json.RawMessage(`{"tag":"` + TagFragment + `","protocol":"freedom","settings":{"fragment":{"packets":"tlshello","length":"100-200","interval":"10-20"}}}`)
	private  = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "fc00::/7", "fe80::/10"}
)

func tweak(out map[string]json.RawMessage, o Options) error {
	t := o.Tweaks
	tun := o.Mode == ModeTun
	if !t.Fragment && !(tun && (len(t.DNS) > 0 || t.NoIPv6 || t.LAN)) {
		return nil
	}
	var outbounds []map[string]json.RawMessage
	if err := json.Unmarshal(out["outbounds"], &outbounds); err != nil || len(outbounds) == 0 {
		return ErrConfig
	}
	var main string
	json.Unmarshal(outbounds[0]["tag"], &main)
	if t.Fragment {
		split := false
		for _, ob := range outbounds {
			if shield(ob) {
				split = true
			}
		}
		if split {
			outbounds = append(outbounds, map[string]json.RawMessage{})
			json.Unmarshal(fragment, &outbounds[len(outbounds)-1])
		}
	}
	var rules []json.RawMessage
	if tun && len(t.DNS) > 0 {
		dns := map[string]json.RawMessage{}
		if len(out["dns"]) > 0 && string(out["dns"]) != "null" && json.Unmarshal(out["dns"], &dns) != nil {
			return ErrConfig
		}
		dns["servers"] = mustJSON(t.DNS)
		if t.NoIPv6 {
			dns["queryStrategy"] = mustJSON("UseIPv4")
		}
		out["dns"] = mustJSON(dns)
		if main != "" {
			rules = append(rules, mustJSON(map[string]any{"type": "field", "ip": t.DNS, "outboundTag": main}))
		}
	} else if tun && t.NoIPv6 && len(out["dns"]) > 0 && string(out["dns"]) != "null" {
		dns := map[string]json.RawMessage{}
		if json.Unmarshal(out["dns"], &dns) != nil {
			return ErrConfig
		}
		dns["queryStrategy"] = mustJSON("UseIPv4")
		out["dns"] = mustJSON(dns)
	}
	if tun && t.NoIPv6 {
		rules = append(rules, mustJSON(map[string]any{"type": "field", "inboundTag": []string{TagTun}, "ip": []string{"::/0"}, "outboundTag": TagBlock}))
		outbounds = append(outbounds, map[string]json.RawMessage{"tag": mustJSON(TagBlock), "protocol": mustJSON("blackhole")})
	}
	if tun && t.LAN {
		rules = append(rules, mustJSON(map[string]any{"type": "field", "inboundTag": []string{TagTun}, "ip": private, "outboundTag": TagLAN}))
		outbounds = append(outbounds, map[string]json.RawMessage{"tag": mustJSON(TagLAN), "protocol": mustJSON("freedom")})
	}
	out["outbounds"] = mustJSON(outbounds)
	if len(rules) == 0 {
		return nil
	}
	routing := map[string]json.RawMessage{}
	if len(out["routing"]) > 0 && json.Unmarshal(out["routing"], &routing) != nil {
		return ErrConfig
	}
	var old []json.RawMessage
	if len(routing["rules"]) > 0 && string(routing["rules"]) != "null" && json.Unmarshal(routing["rules"], &old) != nil {
		return ErrConfig
	}
	routing["rules"] = mustJSON(append(rules, old...))
	out["routing"] = mustJSON(routing)
	return nil
}

func shield(ob map[string]json.RawMessage) bool {
	var protocol string
	json.Unmarshal(ob["protocol"], &protocol)
	if !slices.Contains([]string{"vless", "vmess", "trojan"}, protocol) {
		return false
	}
	stream := map[string]json.RawMessage{}
	if json.Unmarshal(ob["streamSettings"], &stream) != nil {
		return false
	}
	var security, network string
	json.Unmarshal(stream["security"], &security)
	json.Unmarshal(stream["network"], &network)
	if (security != "tls" && security != "reality") || slices.Contains([]string{"hysteria", "kcp", "mkcp", "quic"}, network) {
		return false
	}
	sockopt := map[string]json.RawMessage{}
	if len(stream["sockopt"]) > 0 && json.Unmarshal(stream["sockopt"], &sockopt) != nil {
		return false
	}
	if _, ok := sockopt["dialerProxy"]; ok {
		return false
	}
	sockopt["dialerProxy"] = mustJSON(TagFragment)
	stream["sockopt"] = mustJSON(sockopt)
	ob["streamSettings"] = mustJSON(stream)
	return true
}
