package sub

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

const (
	FormatJSON  = "json"
	FormatLinks = "links"
)

const (
	maxLines = 1000
	maxLine  = 8 << 10
)

var resolvers = map[string]any{
	"servers":       []string{"https://8.8.8.8/dns-query", "https://1.1.1.1/dns-query"},
	"queryStrategy": "UseIPv4",
}

var (
	direct = map[string]any{"tag": "direct", "protocol": "freedom"}
	block  = map[string]any{"tag": "block", "protocol": "blackhole"}
)

var fingerprints = map[string]bool{
	"chrome": true, "firefox": true, "safari": true, "ios": true, "android": true, "edge": true,
	"360": true, "qq": true, "random": true, "randomized": true, "randomizednoalpn": true,
}

var ciphers = map[string]int{
	"aes-128-gcm": 0, "aes-256-gcm": 0,
	"chacha20-poly1305": 0, "chacha20-ietf-poly1305": 0,
	"xchacha20-poly1305": 0, "xchacha20-ietf-poly1305": 0,
	"2022-blake3-aes-128-gcm": 16, "2022-blake3-aes-256-gcm": 32, "2022-blake3-chacha20-poly1305": 32,
}

type target struct {
	user, host, port, name string
	q                      url.Values
}

func ParseLinks(text string) (*Profile, error) {
	return collect(&Profile{Flags: map[string]string{}, Format: FormatLinks}, links(text))
}

func links(text string) []json.RawMessage {
	text = strings.TrimPrefix(text, "\ufeff")
	if len(text) > maxBody {
		return nil
	}
	if !strings.Contains(text, "://") {
		b, ok := unbase64(text)
		if !ok {
			return nil
		}
		text = strings.TrimPrefix(string(b), "\ufeff")
	}
	var out []json.RawMessage
	read := 0
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if read++; read > maxLines {
			break
		}
		if len(line) > maxLine || strings.HasPrefix(line, "#") {
			continue
		}
		if c, ok := link(line); ok {
			out = append(out, c)
		}
	}
	return out
}

func link(line string) (json.RawMessage, bool) {
	scheme, rest, ok := strings.Cut(line, "://")
	if !ok {
		return nil, false
	}
	var t target
	var out map[string]any
	switch strings.ToLower(scheme) {
	case "vless":
		t, out, ok = through(rest, vless)
	case "trojan":
		t, out, ok = through(rest, trojan)
	case "hysteria2", "hy2":
		t, out, ok = through(rest, hysteria)
	case "vmess":
		t, out, ok = vmess(rest)
	case "ss":
		t, out, ok = shadowsocks(rest)
	default:
		return nil, false
	}
	if !ok {
		return nil, false
	}
	name := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, t.name))
	if name == "" {
		name = net.JoinHostPort(t.host, t.port)
	}
	b, err := json.Marshal(map[string]any{"remarks": name, "dns": resolvers, "outbounds": []any{out, direct, block}})
	return b, err == nil
}

func through(rest string, build func(*target) (map[string]any, bool)) (target, map[string]any, bool) {
	t, ok := pieces(rest)
	if !ok {
		return target{}, nil, false
	}
	out, ok := build(&t)
	return t, out, ok
}

func pieces(rest string) (target, bool) {
	rest, frag, _ := strings.Cut(rest, "#")
	rest, query, _ := strings.Cut(rest, "?")
	q, err := url.ParseQuery(query)
	if err != nil {
		return target{}, false
	}
	t := target{q: q, name: unescape(frag)}
	if at := strings.LastIndexByte(rest, '@'); at >= 0 {
		if t.user, err = url.PathUnescape(rest[:at]); err != nil {
			return target{}, false
		}
		rest = rest[at+1:]
	}
	rest, _, _ = strings.Cut(rest, "/")
	var ok bool
	t.host, t.port, ok = hostPort(rest)
	return t, ok && clean(t)
}

func clean(t target) bool {
	control := func(v string) bool { return strings.ContainsFunc(v, unicode.IsControl) }
	if control(t.user) || control(t.host) || control(t.port) {
		return false
	}
	for _, vs := range t.q {
		for _, v := range vs {
			if control(v) {
				return false
			}
		}
	}
	return true
}

func unescape(v string) string {
	if u, err := url.PathUnescape(v); err == nil {
		return u
	}
	return v
}

func hostPort(v string) (host, port string, ok bool) {
	if inner, bracketed := strings.CutPrefix(v, "["); bracketed {
		host, port, ok = strings.Cut(inner, "]")
		if addr, err := netip.ParseAddr(host); !ok || err != nil || !addr.Is6() || addr.Zone() != "" {
			return "", "", false
		}
		if port == "" {
			return host, "", true
		}
		port, ok = strings.CutPrefix(port, ":")
		return host, port, ok
	}
	host, port, _ = strings.Cut(v, ":")
	return host, port, address(host)
}

func address(host string) bool {
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Zone() == ""
	}
	if host == "" || len(host) > 253 {
		return false
	}
	for _, r := range host {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.' || r == '_') {
			return false
		}
	}
	return true
}

func number(v string) (int, bool) {
	if v == "" || len(v) > 5 || strings.Trim(v, "0123456789") != "" {
		return 0, false
	}
	n, _ := strconv.Atoi(v)
	return n, n > 0 && n < 65536
}

func or(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func items(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func fields(kv ...string) map[string]any {
	m := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			m[kv[i]] = kv[i+1]
		}
	}
	return m
}

func proxy(protocol string, settings, stream map[string]any) map[string]any {
	return map[string]any{"tag": "proxy", "protocol": protocol, "settings": settings, "streamSettings": stream}
}

func endpoint(t *target, port int, extra ...string) map[string]any {
	m := fields(extra...)
	m["address"], m["port"] = t.host, port
	return m
}

func vless(t *target) (map[string]any, bool) {
	port, ok := number(t.port)
	enc, flow := or(t.q.Get("encryption"), "none"), t.q.Get("flow")
	s, streamed := stream(t.q, or(t.q.Get("security"), "none"))
	if !ok || !streamed || !uuid(t.user) || !sealed(enc) || (enc == "none" && s["security"] == nil) {
		return nil, false
	}
	switch flow {
	case "":
	case "xtls-rprx-vision", "xtls-rprx-vision-udp443":
		if enc == "none" && s["network"] != "tcp" {
			return nil, false
		}
	default:
		return nil, false
	}
	user := fields("id", t.user, "encryption", enc, "flow", flow)
	server := endpoint(t, port)
	server["users"] = []any{user}
	return proxy("vless", map[string]any{"vnext": []any{server}}, s), true
}

func trojan(t *target) (map[string]any, bool) {
	port, ok := number(t.port)
	s, streamed := stream(t.q, or(t.q.Get("security"), "tls"))
	if !ok || !streamed || t.user == "" || s["security"] == nil || t.q.Get("flow") != "" {
		return nil, false
	}
	return proxy("trojan", map[string]any{"servers": []any{endpoint(t, port, "password", t.user)}}, s), true
}

func vmess(rest string) (target, map[string]any, bool) {
	body, frag, _ := strings.Cut(rest, "#")
	raw, ok := unbase64(body)
	var v map[string]any
	if !ok || json.Unmarshal(raw, &v) != nil || v == nil {
		return target{}, nil, false
	}
	text := func(k string) string {
		switch x := v[k].(type) {
		case string:
			return strings.TrimSpace(x)
		case float64:
			return strconv.FormatFloat(x, 'f', -1, 64)
		}
		return ""
	}
	network, kind := strings.ToLower(or(text("net"), "tcp")), text("type")
	q := url.Values{"type": {network}, "security": {text("tls")}, "host": {text("host")}, "path": {text("path")}, "sni": {text("sni")}, "alpn": {text("alpn")}, "fp": {text("fp")}}
	switch network {
	case "grpc":
		q.Set("serviceName", text("path"))
		q.Set("authority", text("host"))
		q.Set("mode", kind)
	case "xhttp", "splithttp":
		if kind != "none" {
			q.Set("mode", kind)
		}
	default:
		q.Set("headerType", kind)
	}
	t := target{
		user: text("id"),
		host: strings.TrimSuffix(strings.TrimPrefix(text("add"), "["), "]"),
		port: text("port"),
		name: or(text("ps"), unescape(frag)),
		q:    q,
	}
	port, ok := number(t.port)
	security := strings.ToLower(text("scy"))
	switch security {
	case "", "none", "zero":
		security = "auto"
	case "auto", "aes-128-gcm", "chacha20-poly1305":
	default:
		return target{}, nil, false
	}
	s, streamed := stream(q, q.Get("security"))
	if !ok || !streamed || !address(t.host) || !uuid(t.user) || !clean(t) || (text("aid") != "" && text("aid") != "0") {
		return target{}, nil, false
	}
	server := endpoint(&t, port)
	server["users"] = []any{fields("id", t.user, "security", security)}
	return t, proxy("vmess", map[string]any{"vnext": []any{server}}, s), true
}

func shadowsocks(rest string) (target, map[string]any, bool) {
	var t target
	ok := true
	if body, frag, _ := strings.Cut(rest, "#"); !strings.Contains(body, "@") {
		raw, decoded := unbase64(strings.TrimSuffix(body, "/"))
		at := strings.LastIndexByte(string(raw), '@')
		if !decoded || at < 0 {
			return target{}, nil, false
		}
		t = target{user: string(raw[:at]), name: unescape(frag), q: url.Values{}}
		t.host, t.port, ok = hostPort(string(raw[at+1:]))
		ok = ok && clean(t)
	} else {
		t, ok = pieces(rest)
	}
	if !ok {
		return target{}, nil, false
	}
	userinfo := t.user
	if !strings.Contains(userinfo, ":") {
		raw, _ := unbase64(userinfo)
		userinfo = string(raw)
	}
	method, password, _ := strings.Cut(userinfo, ":")
	method = strings.ToLower(method)
	size, known := ciphers[method]
	port, numbered := number(t.port)
	s, streamed := stream(t.q, or(t.q.Get("security"), "none"))
	if !known || !numbered || !streamed || password == "" || strings.ContainsFunc(password, unicode.IsControl) || t.q.Has("plugin") || (size > 0 && !psk(password, size)) {
		return target{}, nil, false
	}
	return t, proxy("shadowsocks", map[string]any{"servers": []any{endpoint(&t, port, "method", method, "password", password)}}, s), true
}

func psk(password string, size int) bool {
	for _, part := range strings.Split(password, ":") {
		b, err := base64.StdEncoding.DecodeString(part)
		if err != nil {
			b = []byte(part)
		}
		if len(b) != size {
			return false
		}
	}
	return true
}

func hysteria(t *target) (map[string]any, bool) {
	t.port = or(t.port, "443")
	ports := or(t.q.Get("mport"), t.port)
	first, hops, ok := spread(ports)
	tls := fields("serverName", t.q.Get("sni"))
	tls["alpn"] = []string{"h3"}
	if !ok || !pin(tls, t.q.Get("pinSHA256")) {
		return nil, false
	}
	var masks []any
	if hops {
		masks = append(masks, map[string]any{"type": "udphop", "settings": map[string]any{"mode": "intervalLocal,intervalRemote", "remotePorts": ports}})
	}
	switch t.q.Get("obfs") {
	case "":
	case "salamander":
		password := t.q.Get("obfs-password")
		if password == "" {
			return nil, false
		}
		masks = append(masks, map[string]any{"type": "salamander", "settings": map[string]any{"password": password}})
	default:
		return nil, false
	}
	s := map[string]any{
		"network":          "hysteria",
		"hysteriaSettings": map[string]any{"version": 2, "auth": t.user},
		"security":         "tls",
		"tlsSettings":      tls,
	}
	if len(masks) > 0 {
		s["finalmask"] = map[string]any{"udp": masks}
	}
	settings := endpoint(t, first)
	settings["version"] = 2
	return proxy("hysteria", settings, s), true
}

func spread(spec string) (first int, hops, ok bool) {
	parts := strings.Split(spec, ",")
	for _, part := range parts {
		low, high, ranged := strings.Cut(part, "-")
		from, okFrom := number(low)
		to, okTo := from, true
		if ranged {
			to, okTo = number(high)
		}
		if !okFrom || !okTo || from > to {
			return 0, false, false
		}
		if first == 0 {
			first = from
		}
		hops = hops || to > from
	}
	return first, hops || len(parts) > 1, true
}

func stream(q url.Values, security string) (map[string]any, bool) {
	s, ok := transport(q)
	return s, ok && secure(s, q, strings.ToLower(security))
}

func transport(q url.Values) (map[string]any, bool) {
	host, path := q.Get("host"), q.Get("path")
	network := strings.ToLower(or(q.Get("type"), "tcp"))
	s := map[string]any{"network": network}
	switch network {
	case "tcp", "raw":
		s["network"] = "tcp"
		switch q.Get("headerType") {
		case "", "none":
		case "http":
			request := map[string]any{"path": items(or(path, "/"))}
			if host != "" {
				request["headers"] = map[string]any{"Host": items(host)}
			}
			s["tcpSettings"] = map[string]any{"header": map[string]any{"type": "http", "request": request}}
		default:
			return nil, false
		}
	case "ws", "httpupgrade":
		s[network+"Settings"] = fields("host", host, "path", path)
	case "grpc":
		g := fields("serviceName", q.Get("serviceName"), "authority", q.Get("authority"))
		g["multiMode"] = q.Get("mode") == "multi"
		s["grpcSettings"] = g
	case "xhttp", "splithttp":
		s["network"] = "xhttp"
		mode := q.Get("mode")
		switch mode {
		case "", "auto", "packet-up", "stream-up", "stream-one":
		default:
			return nil, false
		}
		x := fields("host", host, "path", path, "mode", mode)
		if q.Get("extra") != "" {
			var extra map[string]json.RawMessage
			if json.Unmarshal([]byte(q.Get("extra")), &extra) != nil || extra == nil {
				return nil, false
			}
			x["extra"] = extra
		}
		s["xhttpSettings"] = x
	default:
		return nil, false
	}
	return s, true
}

func secure(s map[string]any, q url.Values, security string) bool {
	sni, fp := q.Get("sni"), strings.ToLower(q.Get("fp"))
	if fp != "" && !fingerprints[fp] && security != "" && security != "none" {
		return false
	}
	switch security {
	case "", "none":
	case "tls":
		if sni == "" {
			sni, _, _ = strings.Cut(or(q.Get("authority"), q.Get("host")), ",")
		}
		tls := fields("serverName", strings.TrimSpace(sni), "fingerprint", fp, "echConfigList", q.Get("ech"))
		if alpn := items(q.Get("alpn")); len(alpn) > 0 {
			tls["alpn"] = alpn
		}
		if names := items(q.Get("vcn")); len(names) > 0 {
			tls["verifyPeerCertByName"] = strings.Join(names, ",")
		}
		s["security"], s["tlsSettings"] = "tls", tls
		return pin(tls, q.Get("pcs"))
	case "reality":
		key, ok := wide(q.Get("pbk"), 32)
		sid, spx := q.Get("sid"), q.Get("spx")
		if network := s["network"]; !ok || network == "ws" || network == "httpupgrade" || !short(sid) || (spx != "" && spx[0] != '/') {
			return false
		}
		r := fields("serverName", sni, "fingerprint", fp, "publicKey", key, "shortId", sid, "spiderX", spx)
		if pqv := q.Get("pqv"); pqv != "" {
			if r["mldsa65Verify"], ok = wide(pqv, 1952); !ok {
				return false
			}
		}
		s["security"], s["realitySettings"] = "reality", r
	default:
		return false
	}
	return true
}

func pin(tls map[string]any, hashes string) bool {
	list := items(hashes)
	for _, h := range list {
		b, err := hex.DecodeString(strings.ReplaceAll(h, ":", ""))
		if err != nil || len(b) != 32 {
			return false
		}
	}
	if len(list) > 0 {
		tls["pinnedPeerCertSha256"] = strings.Join(list, ",")
	}
	return true
}

func wide(v string, size int) (string, bool) {
	b, ok := unbase64(v)
	if !ok || len(b) != size {
		return "", false
	}
	return base64.RawURLEncoding.EncodeToString(b), true
}

func short(sid string) bool {
	_, err := hex.DecodeString(sid)
	return len(sid) <= 16 && err == nil
}

func uuid(id string) bool {
	if n := len(id); n > 0 && n <= 30 {
		return true
	}
	plain := id
	if len(id) == 36 {
		for _, i := range []int{8, 13, 18, 23} {
			if id[i] != '-' {
				return false
			}
		}
		plain = strings.ReplaceAll(id, "-", "")
	}
	_, err := hex.DecodeString(plain)
	return len(plain) == 32 && err == nil
}

func sealed(enc string) bool {
	if enc == "none" {
		return true
	}
	s := strings.Split(enc, ".")
	if len(s) < 4 || s[0] != "mlkem768x25519plus" || (s[1] != "native" && s[1] != "xorpub" && s[1] != "random") || (s[2] != "1rtt" && s[2] != "0rtt") {
		return false
	}
	for _, part := range s[3:] {
		if len(part) < 20 {
			continue
		}
		if b, err := base64.RawURLEncoding.DecodeString(part); err != nil || (len(b) != 32 && len(b) != 1184) {
			return false
		}
	}
	return true
}
