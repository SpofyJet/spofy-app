package sub

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var ErrFormat = errors.New("sub: unsupported subscription format")

type Usage struct {
	Upload   int64     `json:"upload"`
	Download int64     `json:"download"`
	Total    int64     `json:"total"`
	Expire   time.Time `json:"expire"`
}

type Server struct {
	ID       string          `json:"id"`
	Endpoint string          `json:"endpoint"`
	Name     string          `json:"name"`
	Label    string          `json:"label"`
	Country  string          `json:"country"`
	Protocol string          `json:"protocol"`
	Config   json.RawMessage `json:"config"`
}

type Profile struct {
	Title       string            `json:"title"`
	WebPageURL  string            `json:"webPageUrl"`
	SupportURL  string            `json:"supportUrl"`
	Announce    string            `json:"announce"`
	UpdateEvery time.Duration     `json:"updateEvery"`
	Usage       Usage             `json:"usage"`
	Flags       map[string]string `json:"flags"`
	ETag        string            `json:"etag"`
	Servers     []Server          `json:"servers"`
	Notes       []string          `json:"notes,omitempty"`
}

func Parse(body []byte, h http.Header) (*Profile, error) {
	body = bytes.TrimSpace(bytes.TrimPrefix(body, []byte("\xef\xbb\xbf")))
	var raws []json.RawMessage
	switch {
	case len(body) > 0 && body[0] == '[':
		if err := json.Unmarshal(body, &raws); err != nil {
			return nil, ErrFormat
		}
	case len(body) > 0 && body[0] == '{':
		if !json.Valid(body) {
			return nil, ErrFormat
		}
		raws = []json.RawMessage{body}
	default:
		return nil, ErrFormat
	}
	p := &Profile{
		Title:       decode(h.Get("Profile-Title")),
		WebPageURL:  h.Get("Profile-Web-Page-Url"),
		SupportURL:  h.Get("Support-Url"),
		Announce:    decode(h.Get("Announce")),
		UpdateEvery: interval(h.Get("Profile-Update-Interval")),
		Usage:       usage(h.Get("Subscription-Userinfo")),
		Flags:       map[string]string{},
		ETag:        h.Get("Etag"),
	}
	for k, v := range h {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-hwid-") && len(v) > 0 {
			p.Flags[strings.TrimPrefix(lk, "x-")] = v[0]
		}
	}
	seen := map[string]int{}
	for _, raw := range raws {
		s, real, ok := server(raw)
		if !ok {
			continue
		}
		if !real {
			p.Notes = append(p.Notes, s.Name)
			continue
		}
		seen[s.ID]++
		if n := seen[s.ID]; n > 1 {
			s.ID += "-" + strconv.Itoa(n)
		}
		p.Servers = append(p.Servers, s)
	}
	if len(p.Servers) == 0 && len(p.Notes) == 0 {
		return nil, ErrFormat
	}
	return p, nil
}

func decode(v string) string {
	rest, ok := strings.CutPrefix(v, "base64:")
	if !ok {
		return v
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(rest); err == nil {
			return string(b)
		}
	}
	return ""
}

func interval(v string) time.Duration {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || f <= 0 {
		return 0
	}
	return time.Duration(f * float64(time.Hour))
}

func usage(v string) Usage {
	var u Usage
	for _, part := range strings.Split(v, ";") {
		k, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		if err != nil {
			continue
		}
		switch strings.ToLower(k) {
		case "upload":
			u.Upload = n
		case "download":
			u.Download = n
		case "total":
			u.Total = n
		case "expire":
			if n > 0 {
				u.Expire = time.Unix(n, 0).UTC()
			}
		}
	}
	return u
}

type outbound struct {
	Protocol string `json:"protocol"`
	Settings struct {
		Address string `json:"address"`
		Port    int    `json:"port"`
		Vnext   []struct {
			Address string `json:"address"`
			Port    int    `json:"port"`
		} `json:"vnext"`
		Servers []struct {
			Address string `json:"address"`
			Port    int    `json:"port"`
		} `json:"servers"`
	} `json:"settings"`
	Stream struct {
		Network  string `json:"network"`
		Security string `json:"security"`
		Reality  struct {
			ServerName string `json:"serverName"`
		} `json:"realitySettings"`
		TLS struct {
			ServerName string `json:"serverName"`
		} `json:"tlsSettings"`
		XHTTP struct {
			Host string `json:"host"`
			Path string `json:"path"`
		} `json:"xhttpSettings"`
		WS struct {
			Host string `json:"host"`
			Path string `json:"path"`
		} `json:"wsSettings"`
		GRPC struct {
			ServiceName string `json:"serviceName"`
		} `json:"grpcSettings"`
	} `json:"streamSettings"`
}

func server(raw json.RawMessage) (s Server, real, ok bool) {
	var c struct {
		Remarks   string     `json:"remarks"`
		Outbounds []outbound `json:"outbounds"`
	}
	if err := json.Unmarshal(raw, &c); err != nil || len(c.Outbounds) == 0 {
		return Server{}, false, false
	}
	o := c.Outbounds[0]
	addr, port := o.Settings.Address, o.Settings.Port
	if len(o.Settings.Vnext) > 0 {
		addr, port = o.Settings.Vnext[0].Address, o.Settings.Vnext[0].Port
	} else if len(o.Settings.Servers) > 0 {
		addr, port = o.Settings.Servers[0].Address, o.Settings.Servers[0].Port
	}
	if ip := net.ParseIP(addr); ip != nil && ip.IsUnspecified() {
		return Server{Name: strings.TrimSpace(c.Remarks)}, false, true
	}
	endpoint := strings.Join([]string{
		o.Protocol, addr, strconv.Itoa(port), o.Stream.Network, o.Stream.Security,
		o.Stream.Reality.ServerName, o.Stream.TLS.ServerName,
		o.Stream.XHTTP.Host, o.Stream.XHTTP.Path, o.Stream.WS.Host, o.Stream.WS.Path, o.Stream.GRPC.ServiceName,
	}, "|")
	country, label := split(c.Remarks)
	id := key(c.Remarks)
	if id == "" {
		id = digest(endpoint)
	}
	return Server{
		ID:       id,
		Endpoint: digest(endpoint),
		Name:     c.Remarks,
		Label:    label,
		Country:  country,
		Protocol: o.Protocol,
		Config:   raw,
	}, true, true
}

func digest(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:8])
}

func key(name string) string {
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func split(name string) (country, label string) {
	rs := []rune(name)
	for i := 0; i+1 < len(rs); i++ {
		if regional(rs[i]) && regional(rs[i+1]) {
			country = string([]rune{'A' + rs[i] - 0x1F1E6, 'A' + rs[i+1] - 0x1F1E6})
			rs = append(rs[:i:i], rs[i+2:]...)
			break
		}
	}
	return country, strings.Join(strings.Fields(string(rs)), " ")
}

func regional(r rune) bool {
	return r >= 0x1F1E6 && r <= 0x1F1FF
}
