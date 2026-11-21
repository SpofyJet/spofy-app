package sub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/SpofyJet/spofy-app/core/internal/engine"
	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"
)

const testID = "00000000-0000-4000-8000-0000000000a1"

var (
	realityKey = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	mldsaKey   = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{9}, 1952))
	certHash   = strings.TrimSuffix(strings.Repeat("AB:", 32), ":")
	ssKey16    = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 16))
	ssKey32    = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	ssKey32b   = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
	mlkem      = "mlkem768x25519plus.native.0rtt." + realityKey
)

func vmessLink(fields map[string]any, enc *base64.Encoding) string {
	b, _ := json.Marshal(fields)
	return "vmess://" + enc.EncodeToString(b)
}

func b64(enc *base64.Encoding, text string) string {
	return enc.EncodeToString([]byte(text))
}

var (
	tunDesktop = xcfg.Options{Mode: xcfg.ModeTun, Tun: xcfg.Tun{Name: "spofy0", MTU: 1500, Gateway: []string{"172.19.0.1/30"}, Routes: []string{"0.0.0.0/1", "128.0.0.0/1"}}}
	tunPhone   = xcfg.Options{Mode: xcfg.ModeTun, Tun: xcfg.Tun{Name: "spofy0", MTU: 1500, Resolver: []string{"172.19.0.2"}}}
	probing    = xcfg.Options{Mode: xcfg.ModeProbe, LogLevel: "none"}
)

func forkBuild(cfg []byte) (*core.Config, error) {
	c := &conf.Config{}
	if err := json.Unmarshal(cfg, c); err != nil {
		return nil, err
	}
	return c.Build()
}

func forkLoads(t *testing.T, s Server) {
	t.Helper()
	if _, err := forkBuild(s.Config); err != nil {
		t.Fatalf("%s: the fork refuses the config: %v\n%s", s.Name, err, s.Config)
	}
	for _, o := range []xcfg.Options{tunDesktop, tunPhone} {
		built, err := xcfg.Build(s.Config, o)
		if err != nil {
			t.Fatalf("%s: %v", s.Name, err)
		}
		if err := engine.Validate(built); err != nil {
			t.Fatalf("%s: the fork refuses the tun config: %v", s.Name, err)
		}
	}
	probe, err := xcfg.Build(s.Config, probing)
	if err != nil {
		t.Fatalf("%s: %v", s.Name, err)
	}
	built, err := forkBuild(probe)
	if err != nil {
		t.Fatalf("%s: %v", s.Name, err)
	}
	inst, err := core.New(built)
	if err != nil {
		t.Fatalf("%s: the fork cannot make an instance: %v", s.Name, err)
	}
	inst.Close()
	if xcfg.MainTag(s.Config) != "proxy" {
		t.Fatalf("%s: main tag = %q", s.Name, xcfg.MainTag(s.Config))
	}
}

func at(v any, path string) any {
	for _, step := range strings.Split(path, ".") {
		switch x := v.(type) {
		case map[string]any:
			v = x[step]
		case []any:
			i, err := strconv.Atoi(step)
			if err != nil || i >= len(x) {
				return nil
			}
			v = x[i]
		default:
			return nil
		}
	}
	return v
}

type shape struct {
	Remarks   string           `json:"remarks"`
	DNS       map[string]any   `json:"dns"`
	Routing   json.RawMessage  `json:"routing"`
	Inbounds  json.RawMessage  `json:"inbounds"`
	Outbounds []map[string]any `json:"outbounds"`
}

func TestEachLinkBecomesTheConfigItDescribes(t *testing.T) {
	for _, c := range []struct {
		link                    string
		name, country, protocol string
		want                    map[string]any
	}{
		{
			link: "vless://" + testID + "@192.0.2.10:443?type=tcp&security=reality&pbk=" + realityKey + "&fp=chrome&sni=www.example.com&sid=6ba85179e30d4fc2&spx=%2Fsearch&flow=xtls-rprx-vision&encryption=none#%F0%9F%87%A9%F0%9F%87%AA%20Germany%201",
			name: "🇩🇪 Germany 1", country: "DE", protocol: "vless",
			want: map[string]any{
				"settings.vnext.0.address": "192.0.2.10", "settings.vnext.0.port": 443,
				"settings.vnext.0.users.0.id": testID, "settings.vnext.0.users.0.flow": "xtls-rprx-vision", "settings.vnext.0.users.0.encryption": "none",
				"streamSettings.network": "tcp", "streamSettings.security": "reality",
				"streamSettings.realitySettings": map[string]any{"publicKey": realityKey, "serverName": "www.example.com", "fingerprint": "chrome", "shortId": "6ba85179e30d4fc2", "spiderX": "/search"},
			},
		},
		{
			link: "vless://" + testID + "@example.com:443?type=ws&security=tls&host=cdn.example.com&path=%2Fws%3Fed%3D2048&alpn=h2%2Chttp%2F1.1&fp=firefox&allowInsecure=1#WS%20%E2%80%94%20Frankfurt",
			name: "WS — Frankfurt", protocol: "vless",
			want: map[string]any{
				"settings.vnext.0.users.0.flow": nil,
				"streamSettings.wsSettings":     map[string]any{"host": "cdn.example.com", "path": "/ws?ed=2048"},
				"streamSettings.tlsSettings":    map[string]any{"serverName": "cdn.example.com", "alpn": []string{"h2", "http/1.1"}, "fingerprint": "firefox"},
			},
		},
		{
			link: "vless://" + testID + "@[2001:db8::10]:8443?type=grpc&serviceName=svc&authority=grpc.example.com&mode=multi&security=reality&pbk=" + realityKey + "&sni=grpc.example.com&fp=edge#%F0%9F%87%B3%F0%9F%87%B1%20NL",
			name: "🇳🇱 NL", country: "NL", protocol: "vless",
			want: map[string]any{
				"settings.vnext.0.address":       "2001:db8::10",
				"settings.vnext.0.port":          8443,
				"streamSettings.grpcSettings":    map[string]any{"serviceName": "svc", "authority": "grpc.example.com", "multiMode": true},
				"streamSettings.realitySettings": map[string]any{"publicKey": realityKey, "serverName": "grpc.example.com", "fingerprint": "edge"},
				"streamSettings.network":         "grpc",
				"streamSettings.security":        "reality",
			},
		},
		{
			link: "vless://" + testID + "@192.0.2.11:443?type=xhttp&mode=packet-up&host=x.example.com&path=%2Fx&extra=" + url.QueryEscape(`{"xPaddingBytes":"100-1000","noGRPCHeader":true}`) + "&security=tls&sni=x.example.com#XHTTP",
			name: "XHTTP", protocol: "vless",
			want: map[string]any{
				"streamSettings.network":       "xhttp",
				"streamSettings.xhttpSettings": map[string]any{"mode": "packet-up", "host": "x.example.com", "path": "/x", "extra": map[string]any{"xPaddingBytes": "100-1000", "noGRPCHeader": true}},
				"streamSettings.tlsSettings":   map[string]any{"serverName": "x.example.com"},
			},
		},
		{
			link: "vless://" + testID + "@192.0.2.12:443?type=splithttp&security=reality&pbk=" + realityKey + "&pqv=" + mldsaKey + "&sni=www.example.com&sid=ab#PQ",
			name: "PQ", protocol: "vless",
			want: map[string]any{
				"streamSettings.network":                       "xhttp",
				"streamSettings.realitySettings.mldsa65Verify": mldsaKey,
				"streamSettings.realitySettings.shortId":       "ab",
			},
		},
		{
			link: "vless://" + testID + "@192.0.2.13:443?type=httpupgrade&host=up.example.com&path=%2Fup&security=tls&sni=up.example.com&pcs=" + url.QueryEscape(certHash) + "&vcn=a.example.com,b.example.com&ech=" + url.QueryEscape("udp://192.0.2.53") + "#Upgrade",
			name: "Upgrade", protocol: "vless",
			want: map[string]any{
				"streamSettings.httpupgradeSettings": map[string]any{"host": "up.example.com", "path": "/up"},
				"streamSettings.tlsSettings": map[string]any{
					"serverName": "up.example.com", "pinnedPeerCertSha256": certHash,
					"verifyPeerCertByName": "a.example.com,b.example.com", "echConfigList": "udp://192.0.2.53",
				},
			},
		},
		{
			link: "vless://" + testID + "@192.0.2.14:443?type=tcp&headerType=http&host=a.example.com%2Cb.example.com&path=%2Fa%2C%2Fb&security=tls#Header",
			name: "Header", protocol: "vless",
			want: map[string]any{
				"streamSettings.tcpSettings.header": map[string]any{"type": "http", "request": map[string]any{
					"path": []string{"/a", "/b"}, "headers": map[string]any{"Host": []string{"a.example.com", "b.example.com"}},
				}},
				"streamSettings.tlsSettings.serverName": "a.example.com",
			},
		},
		{
			link: "vless://" + testID + "@192.0.2.15:80?type=ws&path=%2F&encryption=" + mlkem + "&flow=xtls-rprx-vision#Encrypted",
			name: "Encrypted", protocol: "vless",
			want: map[string]any{
				"settings.vnext.0.users.0.encryption": mlkem, "settings.vnext.0.users.0.flow": "xtls-rprx-vision",
				"streamSettings.security": nil, "streamSettings.network": "ws",
			},
		},
		{
			link: "VLESS://my-friend@Example.org:443?security=TLS&type=RAW",
			name: "Example.org:443", protocol: "vless",
			want: map[string]any{"settings.vnext.0.users.0.id": "my-friend", "streamSettings.network": "tcp", "streamSettings.security": "tls"},
		},
		{
			link: vmessLink(map[string]any{"v": "2", "ps": "🇫🇮 Helsinki", "add": "example.net", "port": "443", "id": testID, "aid": "0", "scy": "auto", "net": "ws", "type": "none", "host": "cdn.example.net", "path": "/vm", "tls": "tls", "sni": "", "alpn": "h2", "fp": "chrome"}, base64.StdEncoding),
			name: "🇫🇮 Helsinki", country: "FI", protocol: "vmess",
			want: map[string]any{
				"settings.vnext.0.address":  "example.net",
				"settings.vnext.0.users.0":  map[string]any{"id": testID, "security": "auto"},
				"streamSettings.wsSettings": map[string]any{"host": "cdn.example.net", "path": "/vm"},
				"streamSettings.tlsSettings": map[string]any{
					"serverName": "cdn.example.net", "alpn": []string{"h2"}, "fingerprint": "chrome",
				},
			},
		},
		{
			link: vmessLink(map[string]any{"v": 2, "ps": "Plain", "add": "2001:db8::20", "port": 10086, "id": testID, "aid": 0, "scy": "none", "net": "tcp", "type": "none", "tls": ""}, base64.RawURLEncoding),
			name: "Plain", protocol: "vmess",
			want: map[string]any{
				"settings.vnext.0.address": "2001:db8::20", "settings.vnext.0.port": 10086,
				"settings.vnext.0.users.0.security": "auto", "streamSettings": map[string]any{"network": "tcp"},
			},
		},
		{
			link: vmessLink(map[string]any{"ps": "gRPC", "add": "192.0.2.21", "port": "443", "id": testID, "aid": "0", "net": "grpc", "type": "multi", "path": "svc", "host": "g.example.net", "tls": "tls"}, base64.URLEncoding) + "#ignored",
			name: "gRPC", protocol: "vmess",
			want: map[string]any{
				"streamSettings.grpcSettings":           map[string]any{"serviceName": "svc", "authority": "g.example.net", "multiMode": true},
				"streamSettings.tlsSettings.serverName": "g.example.net",
			},
		},
		{
			link: "trojan://p%40ss%3Aword@example.org:443?sni=t.example.org#Trojan%20%231",
			name: "Trojan #1", protocol: "trojan",
			want: map[string]any{
				"settings.servers.0": map[string]any{"address": "example.org", "port": 443, "password": "p@ss:word"},
				"streamSettings":     map[string]any{"network": "tcp", "security": "tls", "tlsSettings": map[string]any{"serverName": "t.example.org"}},
			},
		},
		{
			link: "trojan://secret@192.0.2.22:443?type=grpc&serviceName=tj&security=reality&pbk=" + realityKey + "&sni=www.example.com&fp=safari#TJ",
			name: "TJ", protocol: "trojan",
			want: map[string]any{
				"streamSettings.grpcSettings":    map[string]any{"serviceName": "tj", "multiMode": false},
				"streamSettings.realitySettings": map[string]any{"publicKey": realityKey, "serverName": "www.example.com", "fingerprint": "safari"},
			},
		},
		{
			link: "ss://" + b64(base64.RawURLEncoding, "aes-256-gcm:s3cr3t?>") + "@192.0.2.30:8388#SS",
			name: "SS", protocol: "shadowsocks",
			want: map[string]any{
				"settings.servers.0": map[string]any{"address": "192.0.2.30", "port": 8388, "method": "aes-256-gcm", "password": "s3cr3t?>"},
				"streamSettings":     map[string]any{"network": "tcp"},
			},
		},
		{
			link: "ss://" + b64(base64.StdEncoding, "chacha20-ietf-poly1305:??>>") + "@192.0.2.33:8388/?outline=1#Std",
			name: "Std", protocol: "shadowsocks",
			want: map[string]any{"settings.servers.0.password": "??>>", "settings.servers.0.method": "chacha20-ietf-poly1305"},
		},
		{
			link: "ss://2022-blake3-aes-128-gcm:" + url.QueryEscape(ssKey16) + "@[2001:db8::30]:8388/?#SS%202022",
			name: "SS 2022", protocol: "shadowsocks",
			want: map[string]any{"settings.servers.0": map[string]any{"address": "2001:db8::30", "port": 8388, "method": "2022-blake3-aes-128-gcm", "password": ssKey16}},
		},
		{
			link: "ss://" + b64(base64.StdEncoding, "chacha20-ietf-poly1305:p@ss#1@192.0.2.31:8389") + "#Legacy",
			name: "Legacy", protocol: "shadowsocks",
			want: map[string]any{"settings.servers.0": map[string]any{"address": "192.0.2.31", "port": 8389, "method": "chacha20-ietf-poly1305", "password": "p@ss#1"}},
		},
		{
			link: "ss://2022-blake3-aes-256-gcm:" + url.QueryEscape(ssKey32+":"+ssKey32b) + "@192.0.2.32:443#Relay",
			name: "Relay", protocol: "shadowsocks",
			want: map[string]any{"settings.servers.0.password": ssKey32 + ":" + ssKey32b},
		},
		{
			link: "hysteria2://letmein@example.com:443?sni=hy.example.com&obfs=salamander&obfs-password=gawrgura&pinSHA256=" + url.QueryEscape(certHash) + "#%F0%9F%87%BA%F0%9F%87%B8%20US",
			name: "🇺🇸 US", country: "US", protocol: "hysteria",
			want: map[string]any{
				"settings": map[string]any{"address": "example.com", "port": 443, "version": 2},
				"streamSettings": map[string]any{
					"network": "hysteria", "security": "tls",
					"hysteriaSettings": map[string]any{"version": 2, "auth": "letmein"},
					"tlsSettings":      map[string]any{"serverName": "hy.example.com", "alpn": []string{"h3"}, "pinnedPeerCertSha256": certHash},
					"finalmask":        map[string]any{"udp": []any{map[string]any{"type": "salamander", "settings": map[string]any{"password": "gawrgura"}}}},
				},
			},
		},
		{
			link: "hy2://user:pass@192.0.2.40:20000-50000?sni=hy.example.com&insecure=1#Hop",
			name: "Hop", protocol: "hysteria",
			want: map[string]any{
				"settings.port":                           20000,
				"streamSettings.hysteriaSettings.auth":    "user:pass",
				"streamSettings.tlsSettings":              map[string]any{"serverName": "hy.example.com", "alpn": []string{"h3"}},
				"streamSettings.finalmask.udp.0.type":     "udphop",
				"streamSettings.finalmask.udp.0.settings": map[string]any{"mode": "intervalLocal,intervalRemote", "remotePorts": "20000-50000"},
			},
		},
		{
			link: "hy2://k@192.0.2.42:443?mport=1000-1100,2000&obfs=salamander&obfs-password=x#M",
			name: "M", protocol: "hysteria",
			want: map[string]any{
				"settings.port": 1000,
				"streamSettings.finalmask.udp.0.settings": map[string]any{"mode": "intervalLocal,intervalRemote", "remotePorts": "1000-1100,2000"},
				"streamSettings.finalmask.udp.1.type":     "salamander",
			},
		},
		{
			link: "hysteria2://[2001:db8::41]/?sni=hy.example.com",
			name: "[2001:db8::41]:443", protocol: "hysteria",
			want: map[string]any{"settings.address": "2001:db8::41", "settings.port": 443, "streamSettings.finalmask": nil},
		},
	} {
		p, err := ParseLinks(c.link)
		if err != nil {
			t.Fatalf("%s: %v", c.link, err)
		}
		if len(p.Servers) != 1 || p.Format != FormatLinks {
			t.Fatalf("%s: servers %d, format %q", c.link, len(p.Servers), p.Format)
		}
		s := p.Servers[0]
		if s.Name != c.name || s.Country != c.country || s.Protocol != c.protocol || s.ID == "" || s.Endpoint == "" {
			t.Fatalf("%s: server = %+v", c.link, s)
		}
		var cfg shape
		if err := json.Unmarshal(s.Config, &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.Remarks != c.name || len(cfg.Outbounds) != 3 || cfg.Routing != nil || cfg.Inbounds != nil {
			t.Fatalf("%s: config = %s", c.link, s.Config)
		}
		if fmt.Sprint(cfg.Outbounds[1]) != fmt.Sprint(map[string]any{"tag": "direct", "protocol": "freedom"}) || fmt.Sprint(cfg.Outbounds[2]) != fmt.Sprint(map[string]any{"tag": "block", "protocol": "blackhole"}) {
			t.Fatalf("%s: outbounds = %v", c.link, cfg.Outbounds)
		}
		if got, _ := json.Marshal(cfg.DNS); string(got) != `{"queryStrategy":"UseIPv4","servers":["https://8.8.8.8/dns-query","https://1.1.1.1/dns-query"]}` {
			t.Fatalf("%s: dns = %s", c.link, got)
		}
		main := map[string]any(cfg.Outbounds[0])
		if main["tag"] != "proxy" || main["protocol"] != c.protocol {
			t.Fatalf("%s: main outbound = %v", c.link, main)
		}
		for path, want := range c.want {
			got, _ := json.Marshal(at(main, path))
			exp, _ := json.Marshal(want)
			if string(got) != string(exp) {
				t.Fatalf("%s: %s = %s, want %s", c.name, path, got, exp)
			}
		}
		if strings.Contains(string(s.Config), "allowInsecure") || strings.Contains(string(s.Config), "insecure") {
			t.Fatalf("%s: certificate checks are off: %s", c.name, s.Config)
		}
		forkLoads(t, s)
	}
}

func TestLinksTheForkCannotCarryAreSkipped(t *testing.T) {
	reality := "&security=reality&pbk=" + realityKey + "&sni=www.example.com"
	for _, link := range []string{
		"vless://" + testID + "@192.0.2.1:443?type=tcp#plain",
		"vless://" + testID + "@192.0.2.1:443?security=none&encryption=none",
		"vless://" + testID + "@192.0.2.1:443?type=kcp&security=tls",
		"vless://" + testID + "@192.0.2.1:443?type=quic&security=tls",
		"vless://" + testID + "@192.0.2.1:443?type=h2&security=tls",
		"vless://" + testID + "@192.0.2.1:443?type=http&security=tls",
		"vless://" + testID + "@192.0.2.1:443?security=xtls",
		"vless://" + testID + "@192.0.2.1:443?type=ws" + reality,
		"vless://" + testID + "@192.0.2.1:443?type=httpupgrade" + reality,
		"vless://" + testID + "@192.0.2.1:443?security=reality&sni=www.example.com",
		"vless://" + testID + "@192.0.2.1:443?security=reality&pbk=" + realityKey[:40],
		"vless://" + testID + "@192.0.2.1:443?sid=abc" + reality,
		"vless://" + testID + "@192.0.2.1:443?sid=zz" + reality,
		"vless://" + testID + "@192.0.2.1:443?sid=00112233445566778899" + reality,
		"vless://" + testID + "@192.0.2.1:443?spx=search" + reality,
		"vless://" + testID + "@192.0.2.1:443?pqv=" + realityKey + reality,
		"vless://" + testID + "@192.0.2.1:443?fp=hellogolang" + reality,
		"vless://" + testID + "@192.0.2.1:443?fp=unsafe&security=tls",
		"vless://" + testID + "@192.0.2.1:443?type=ws&security=tls&flow=xtls-rprx-vision",
		"vless://" + testID + "@192.0.2.1:443?type=xhttp&flow=xtls-rprx-vision" + reality,
		"vless://" + testID + "@192.0.2.1:443?flow=xtls-rprx-direct&security=tls",
		"vless://" + testID + "@192.0.2.1:443?encryption=aes-128-gcm&security=tls",
		"vless://" + testID + "@192.0.2.1:443?encryption=mlkem768x25519plus.native.0rtt.AAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"vless://" + testID + "@192.0.2.1:443?security=tls&pcs=abcd",
		"vless://" + testID + "@192.0.2.1:443?type=xhttp&mode=stream-two&security=tls",
		"vless://" + testID + "@192.0.2.1:443?type=xhttp&extra=%7Bbroken&security=tls",
		"vless://" + testID + "@192.0.2.1:443?type=xhttp&extra=%5B1%5D&security=tls",
		"vless://" + testID + "@192.0.2.1:443?type=xhttp&extra=null&security=tls",
		"vless://" + testID + "@192.0.2.1:443?type=tcp&headerType=srtp&security=tls",
		"vless://" + testID + "@192.0.2.1:443?security=tls&path=%zz",
		"vless://" + testID + "@192.0.2.1:443?security=tls&host=a%0Ab.example.com",
		"vless://" + testID + "x@192.0.2.1:443?security=tls",
		"vless://0000000000000000000000000000000@192.0.2.1:443?security=tls",
		"vless://" + strings.ReplaceAll(testID, "-", "+") + "@192.0.2.1:443?security=tls",
		"vless://@192.0.2.1:443?security=tls",
		"vless://" + testID + "@192.0.2.1?security=tls",
		"vless://" + testID + "@192.0.2.1:0?security=tls",
		"vless://" + testID + "@192.0.2.1:70000?security=tls",
		"vless://" + testID + "@192.0.2.1:+443?security=tls",
		"vless://" + testID + "@exa mple.com:443?security=tls",
		"vless://" + testID + "@2001:db8::1:443?security=tls",
		"vless://" + testID + "@[2001:db8::1:443?security=tls",
		"vless://" + testID + "@[192.0.2.1]:443?security=tls",
		"vless://" + testID + "@[fe80::1%25eth0]:443?security=tls",
		vmessLink(map[string]any{"add": "192.0.2.1", "port": "443", "id": testID, "aid": "64", "net": "tcp"}, base64.StdEncoding),
		vmessLink(map[string]any{"add": "192.0.2.1", "port": "443", "id": testID, "net": "kcp"}, base64.StdEncoding),
		vmessLink(map[string]any{"add": "192.0.2.1", "port": "443", "id": testID, "scy": "rc4"}, base64.StdEncoding),
		vmessLink(map[string]any{"add": "192.0.2.1", "port": "443", "id": "nope-" + testID}, base64.StdEncoding),
		vmessLink(map[string]any{"add": "192.0.2.1", "id": testID}, base64.StdEncoding),
		"vmess://" + b64(base64.StdEncoding, `[{"add":"192.0.2.1"}]`),
		"vmess://" + b64(base64.StdEncoding, `null`),
		"vmess://%%%",
		"trojan://@example.org:443",
		"trojan://secret@example.org:443?security=none",
		"trojan://secret@example.org:443?flow=xtls-rprx-vision",
		"ss://" + b64(base64.RawURLEncoding, "aes-256-gcm:pw") + "@192.0.2.30:8388/?plugin=obfs-local%3Bobfs%3Dhttp",
		"ss://" + b64(base64.RawURLEncoding, "rc4-md5:pw") + "@192.0.2.30:8388",
		"ss://" + b64(base64.RawURLEncoding, "none:pw") + "@192.0.2.30:8388",
		"ss://" + b64(base64.RawURLEncoding, "aes-256-gcm:") + "@192.0.2.30:8388",
		"ss://" + b64(base64.RawURLEncoding, "aes-256-gcm") + "@192.0.2.30:8388",
		"ss://2022-blake3-aes-256-gcm:" + url.QueryEscape(ssKey16) + "@192.0.2.30:8388",
		"ss://2022-blake3-aes-128-gcm:" + url.QueryEscape(ssKey16+":short") + "@192.0.2.30:8388",
		"ss://" + b64(base64.StdEncoding, "aes-256-gcm:pw-without-host"),
		"ss://!!!!",
		"hysteria2://auth@example.com:443?obfs=gecko&obfs-password=x",
		"hysteria2://auth@example.com:443?obfs=salamander",
		"hysteria2://auth@example.com:500-100",
		"hysteria2://auth@example.com:443,",
		"hysteria2://auth@example.com:443?mport=1-2-3",
		"hysteria2://auth@example.com:443?pinSHA256=deadbeef",
		"hysteria://auth@example.com:443",
		"tuic://" + testID + ":pw@example.com:443",
		"socks://dXNlcjpwYXNz@192.0.2.1:1080#Socks",
		"wireguard://key@192.0.2.1:51820",
		"https://example.com/sub",
		"vless:/" + testID + "@192.0.2.1:443?security=tls",
		"just words",
	} {
		if p, err := ParseLinks(link); !errors.Is(err, ErrFormat) {
			var names []string
			if p != nil {
				for _, s := range p.Servers {
					names = append(names, string(s.Config))
				}
			}
			t.Fatalf("%s was taken: %v", link, names)
		}
	}
}

var trio = []string{
	"vless://" + testID + "@192.0.2.10:443?type=tcp&security=reality&pbk=" + realityKey + "&sni=www.example.com&flow=xtls-rprx-vision#%F0%9F%87%A9%F0%9F%87%AA%20(DE)",
	"trojan://secret@example.org:443?type=ws&host=cdn.example.org&path=%2Ftj#%F0%9F%87%B1%F0%9F%87%B9%20(LT)",
	"hy2://letmein@192.0.2.40:443?sni=hy.example.com&obfs=salamander&obfs-password=pw#%F0%9F%87%AB%F0%9F%87%AE%20(FL)",
}

func breaks(v string) string {
	var b strings.Builder
	for len(v) > 76 {
		b.WriteString(v[:76] + "\r\n")
		v = v[76:]
	}
	return b.String() + v + "\n"
}

func TestALinkListIsReadInEveryWrapping(t *testing.T) {
	plain := strings.Join(trio, "\n")
	want, err := ParseLinks(plain)
	if err != nil {
		t.Fatal(err)
	}
	if len(want.Servers) != 3 || want.Servers[0].ID != "de" || want.Servers[1].ID != "lt" || want.Servers[2].ID != "fl" {
		t.Fatalf("servers = %+v", want.Servers)
	}
	messy := "\ufeff# Provider: Example\r\n\r\n  " + strings.Join(trio, "\r\n\r\n") + "\r\n# end\r\n"
	for name, body := range map[string]string{
		"plain":                plain,
		"messy":                messy,
		"standard":             b64(base64.StdEncoding, plain),
		"standard with breaks": breaks(b64(base64.StdEncoding, messy)),
		"standard unpadded":    b64(base64.RawStdEncoding, messy),
		"url padded":           b64(base64.URLEncoding, messy),
		"url unpadded":         "\ufeff" + b64(base64.RawURLEncoding, messy) + "\n",
	} {
		for _, p := range func() []*Profile {
			a, err := Parse([]byte(body), http.Header{})
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			b, err := ParseLinks(body)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			return []*Profile{a, b}
		}() {
			if p.Format != FormatLinks || !reflect.DeepEqual(p.Servers, want.Servers) {
				t.Fatalf("%s: got %+v", name, p.Servers)
			}
		}
	}
	for _, s := range want.Servers {
		forkLoads(t, s)
	}
}

func TestAListKeepsWhatWorksAndSkipsTheRest(t *testing.T) {
	body := strings.Join([]string{
		"socks://dXNlcjpwYXNz@192.0.2.1:1080#Socks",
		trio[0],
		"vless://broken",
		"not a link at all",
		"tuic://" + testID + ":pw@example.com:443#TUIC",
		trio[1],
		"ss://" + b64(base64.RawURLEncoding, "aes-256-gcm:pw") + "@192.0.2.30:8388/?plugin=v2ray-plugin#Plugin",
		"https://example.com/",
		trio[2],
		"vless://" + testID + "@192.0.2.1:443#NoSecurity",
	}, "\n")
	p, err := Parse([]byte(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range p.Servers {
		ids = append(ids, s.ID)
	}
	if !reflect.DeepEqual(ids, []string{"de", "lt", "fl"}) {
		t.Fatalf("ids = %v", ids)
	}
}

func TestServersFromLinksAreNamedLikeThoseFromThePanel(t *testing.T) {
	body := strings.Join([]string{
		"trojan://a@192.0.2.1:443#%F0%9F%87%A9%F0%9F%87%AA%20Germany",
		"trojan://b@192.0.2.2:443#%F0%9F%87%A9%F0%9F%87%AA%20Germany%20%F0%9F%94%A5",
		"trojan://c@192.0.2.3:443",
		"trojan://d@[2001:db8::4]:443#%20%09",
		"trojan://e@0.0.0.0:1#%E2%8F%B0%20%D0%9F%D0%BE%D0%B4%D0%BF%D0%B8%D1%81%D0%BA%D0%B0%20%D0%B7%D0%B0%D0%BA%D0%BE%D0%BD%D1%87%D0%B8%D0%BB%D0%B0%D1%81%D1%8C",
		"trojan://f@192.0.2.6:443#line%0Abreak",
		"trojan://g@192.0.2.7:443#100%",
	}, "\n")
	p, err := ParseLinks(body)
	if err != nil {
		t.Fatal(err)
	}
	type named struct{ ID, Name, Label, Country string }
	var got []named
	for _, s := range p.Servers {
		got = append(got, named{s.ID, s.Name, s.Label, s.Country})
	}
	want := []named{
		{"germany", "🇩🇪 Germany", "Germany", "DE"},
		{"germany-2", "🇩🇪 Germany 🔥", "Germany 🔥", "DE"},
		{"192023443", "192.0.2.3:443", "192.0.2.3:443", ""},
		{"2001db84443", "[2001:db8::4]:443", "[2001:db8::4]:443", ""},
		{"linebreak", "line break", "line break", ""},
		{"100", "100%", "100%", ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	if !reflect.DeepEqual(p.Notes, []string{"⏰ Подписка закончилась"}) {
		t.Fatalf("notes = %q", p.Notes)
	}
	if p.Servers[0].Endpoint == p.Servers[1].Endpoint {
		t.Fatal("two servers share an endpoint")
	}
}

func TestParseTellsWhichFormatCame(t *testing.T) {
	body, h := fixture(t)
	p, err := Parse(body, h)
	if err != nil || p.Format != FormatJSON {
		t.Fatalf("format %q, err %v", p.Format, err)
	}
	p, err = Parse([]byte(b64(base64.StdEncoding, strings.Join(trio, "\n"))), h)
	if err != nil {
		t.Fatal(err)
	}
	if p.Format != FormatLinks || len(p.Servers) != 3 || p.Title != "🛡 Spofy VPN" || p.Usage.Total != 107374182400 || p.ETag != h.Get("Etag") || p.Flags["hwid-active"] != "true" {
		t.Fatalf("profile = %+v", p)
	}
}

func TestALinkListStopsAtItsLimits(t *testing.T) {
	var many []string
	for i := range maxLines + 5 {
		many = append(many, "trojan://pw@192.0.2.1:"+strconv.Itoa(1000+i)+"#n"+strconv.Itoa(i))
	}
	p, err := ParseLinks(strings.Join(many, "\n\n"))
	if err != nil || len(p.Servers) != maxLines || p.Servers[maxLines-1].Name != "n999" {
		t.Fatalf("err %v, servers %d", err, len(p.Servers))
	}
	p, err = ParseLinks("# one\n" + strings.Join(many[:maxLines], "\n"))
	if err != nil || len(p.Servers) != maxLines-1 {
		t.Fatalf("a comment is a line too: err %v, servers %d", err, len(p.Servers))
	}
	head := "trojan://pw@192.0.2.1:443#"
	fits := head + strings.Repeat("a", maxLine-len(head))
	if p, err := ParseLinks(fits); err != nil || len(p.Servers) != 1 {
		t.Fatalf("a line of %d bytes: %v", len(fits), err)
	}
	if _, err := ParseLinks(fits + "a"); !errors.Is(err, ErrFormat) {
		t.Fatalf("a line of %d bytes: %v", len(fits)+1, err)
	}
	blank := strings.Repeat("\n", 4<<20) + trio[0]
	if p, err := ParseLinks(blank); err != nil || len(p.Servers) != 1 {
		t.Fatalf("blank lines: %v", err)
	}
	if _, err := ParseLinks(trio[0] + strings.Repeat(" ", maxBody)); !errors.Is(err, ErrFormat) {
		t.Fatalf("a text over %d bytes: %v", maxBody, err)
	}
}

func mutate(r *rand.Rand, v []byte) []byte {
	const alphabet = ":/@?#&=%[],.-_+ 0123456789abcdefABCDEF"
	for range 1 + r.IntN(4) {
		var c byte
		if r.IntN(3) == 0 {
			c = byte(r.UintN(256))
		} else {
			c = alphabet[r.IntN(len(alphabet))]
		}
		i := r.IntN(len(v) + 1)
		switch r.IntN(3) {
		case 0:
			v = append(v[:i:i], append([]byte{c}, v[i:]...)...)
		case 1:
			if i < len(v) {
				v = append(v[:i:i], v[i+1:]...)
			}
		default:
			if i < len(v) {
				v = append(v[:i:i], append([]byte{c}, v[i+1:]...)...)
			}
		}
	}
	return v
}

func seeds() []string {
	out := append([]string{}, trio...)
	out = append(out,
		"vless://"+testID+"@[2001:db8::10]:8443?type=grpc&serviceName=svc&authority=g.example.com&mode=multi&security=reality&pbk="+realityKey+"&sni=g.example.com&fp=edge&sid=ab12#G",
		"vless://"+testID+"@192.0.2.13:443?type=httpupgrade&host=up.example.com&path=%2Fup&security=tls&alpn=h2&pcs="+certHash+"#U",
		"vless://"+testID+"@192.0.2.15:80?type=ws&path=%2F&encryption="+mlkem+"&flow=xtls-rprx-vision#E",
		"vless://"+testID+"@192.0.2.14:443?type=tcp&headerType=http&host=a.example.com&path=%2Fa&security=tls#H",
		vmessLink(map[string]any{"ps": "V", "add": "example.net", "port": "443", "id": testID, "aid": "0", "net": "ws", "host": "cdn.example.net", "path": "/vm", "tls": "tls", "fp": "chrome"}, base64.StdEncoding),
		"ss://"+b64(base64.RawURLEncoding, "aes-256-gcm:secret")+"@192.0.2.30:8388#S",
		"ss://2022-blake3-aes-128-gcm:"+url.QueryEscape(ssKey16)+"@192.0.2.30:8388#S2",
		"ss://"+b64(base64.StdEncoding, "chacha20-ietf-poly1305:p@ss@192.0.2.31:8389")+"#L",
		"hysteria2://a:b@example.com:20000-20100,30000?sni=hy.example.com&obfs=salamander&obfs-password=pw&pinSHA256="+certHash+"#Y",
	)
	return out
}

func TestRandomInputNeverBreaksTheParserAndNeverGivesWhatTheForkRefuses(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	list := seeds()
	taken := 0
	for i := range 4000 {
		var in []byte
		switch i % 4 {
		case 0:
			in = make([]byte, r.IntN(400))
			for j := range in {
				in[j] = byte(r.UintN(256))
			}
		case 1:
			pair := list[r.IntN(len(list))] + "\r\n" + list[r.IntN(len(list))]
			in = []byte(b64(base64.StdEncoding, string(mutate(r, []byte(pair)))))
		default:
			in = mutate(r, []byte(list[r.IntN(len(list))]))
		}
		p, err := Parse(in, nil)
		if err != nil {
			if !errors.Is(err, ErrFormat) {
				t.Fatalf("%q: %v", in, err)
			}
			continue
		}
		for _, s := range p.Servers {
			taken++
			if _, err := forkBuild(s.Config); err != nil {
				t.Fatalf("%q gave a config the fork refuses: %v\n%s", in, err, s.Config)
			}
			if _, err := xcfg.Build(s.Config, tunPhone); err != nil {
				t.Fatalf("%q: %v", in, err)
			}
		}
	}
	if taken < 1000 {
		t.Fatalf("only %d servers came out of the mutations, the test checks too little", taken)
	}
}

func FuzzParseLinks(f *testing.F) {
	for _, s := range seeds() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, text string) {
		p, err := ParseLinks(text)
		if err != nil {
			return
		}
		for _, s := range p.Servers {
			if !json.Valid(s.Config) {
				t.Fatalf("%q: %s", text, s.Config)
			}
			if _, err := xcfg.Build(s.Config, tunPhone); err != nil {
				t.Fatalf("%q: %v", text, err)
			}
		}
	})
}
