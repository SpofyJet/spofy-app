package engine

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"
	"github.com/xtls/xray-core/proxy/freedom"
	"github.com/xtls/xray-core/transport/internet"
	"google.golang.org/protobuf/proto"
)

func TestTheCoreFindsNothingDeprecatedInTheSubscription(t *testing.T) {
	var mu sync.Mutex
	var said []string
	SetLogSink(func(line string) {
		mu.Lock()
		said = append(said, line)
		mu.Unlock()
	})
	defer SetLogSink(nil)
	tun := xcfg.Options{Mode: xcfg.ModeTun, Tun: xcfg.Tun{Name: "spofy0", MTU: 1500, Resolver: []string{"172.19.0.2"}}}
	tweaked := tun
	tweaked.Tweaks = xcfg.Tweaks{Fragment: true, NoIPv6: true, LAN: true}
	for _, path := range []string{"../../testdata/sub/sub.json", "../../../testdata/private/sub.json"} {
		for i, raw := range fixtureServers(t, path) {
			for _, o := range []xcfg.Options{tun, tweaked, {Mode: xcfg.ModeProbe, Tweaks: xcfg.Tweaks{Fragment: true}}} {
				cfg, err := xcfg.Build(raw, o)
				if err != nil {
					t.Fatal(err)
				}
				mu.Lock()
				said = said[:0]
				mu.Unlock()
				if err := Validate(cfg); err != nil {
					t.Fatalf("%s #%d: %v", path, i, err)
				}
				mu.Lock()
				for _, line := range said {
					if strings.Contains(line, "migrated") {
						t.Errorf("%s #%d %+v: %s", path, i, o.Tweaks, line)
					}
				}
				mu.Unlock()
			}
		}
	}
}

func rebuilt(t *testing.T, outbound string) map[string]json.RawMessage {
	t.Helper()
	b, err := xcfg.Build(json.RawMessage(`{"outbounds":[{"tag":"p","protocol":"blackhole"},`+outbound+`]}`), xcfg.Options{Mode: xcfg.ModeProbe})
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

func TestTheNameHandlerInTheNewFormIsTheSameHandler(t *testing.T) {
	build := func(settings json.RawMessage) proto.Message {
		t.Helper()
		var c conf.DNSOutboundConfig
		if err := json.Unmarshal(settings, &c); err != nil {
			t.Fatal(err)
		}
		m, err := c.Build()
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	for _, settings := range []string{
		`{"nonIPQuery":"reject","blockTypes":[28]}`,
		`{"nonIPQuery":"drop","blockTypes":[28,65,0]}`,
		`{"nonIPQuery":"skip","blockTypes":[65535]}`,
		`{"blockTypes":[1,28]}`,
		`{"nonIPQuery":"","address":"1.1.1.1","port":53}`,
	} {
		now := rebuilt(t, `{"tag":"d","protocol":"dns","settings":`+settings+`}`)["settings"]
		if old, got := build(json.RawMessage(settings)), build(now); !proto.Equal(old, got) {
			t.Errorf("%s -> %s:\n old %v\n new %v", settings, now, old, got)
		}
	}
}

func TestDirectInTheNewFormDialsTheSameWay(t *testing.T) {
	build := func(raw []byte) (*freedom.Config, *internet.MemoryStreamConfig) {
		t.Helper()
		var c conf.OutboundDetourConfig
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatal(err)
		}
		h, err := c.Build()
		if err != nil {
			t.Fatal(err)
		}
		return parts(t, h)
	}
	for _, outbound := range []string{
		`{"tag":"f","protocol":"freedom","settings":{"domainStrategy":"UseIPv4"}}`,
		`{"tag":"f","protocol":"freedom","settings":{"targetStrategy":"ForceIPv6v4","domainStrategy":"UseIPv4","redirect":"1.1.1.1:53"}}`,
		`{"tag":"f","protocol":"freedom","targetStrategy":"UseIP","settings":{"domainStrategy":"UseIPv4"},"streamSettings":{"sockopt":{"mark":7,"domainStrategy":"UseIPv6"}}}`,
	} {
		now, err := json.Marshal(rebuilt(t, outbound))
		if err != nil {
			t.Fatal(err)
		}
		oldCfg, oldStream := build([]byte(outbound))
		newCfg, newStream := build(now)
		oldCfg.DomainStrategy = internet.DomainStrategy_AS_IS
		if !proto.Equal(oldCfg, newCfg) {
			t.Errorf("%s: freedom %v, now %v", outbound, oldCfg, newCfg)
		}
		eyeballs := func(s *internet.SocketConfig) bool {
			return s.HappyEyeballs != nil && s.HappyEyeballs.TryDelayMs > 0 && s.HappyEyeballs.MaxConcurrentTry > 0
		}
		if oldStream.ProtocolName != newStream.ProtocolName || oldStream.SocketSettings.DomainStrategy != newStream.SocketSettings.DomainStrategy ||
			oldStream.SocketSettings.Mark != newStream.SocketSettings.Mark || eyeballs(oldStream.SocketSettings) || eyeballs(newStream.SocketSettings) ||
			newStream.SecurityType != "" || newStream.FinalMask != nil {
			t.Errorf("%s -> %s: dial %+v, now %+v", outbound, now, oldStream, newStream)
		}
	}
}

func parts(t *testing.T, h *core.OutboundHandlerConfig) (*freedom.Config, *internet.MemoryStreamConfig) {
	t.Helper()
	f, err := h.ProxySettings.GetInstance()
	if err != nil {
		t.Fatal(err)
	}
	s, err := h.SenderSettings.GetInstance()
	if err != nil {
		t.Fatal(err)
	}
	m, err := internet.ToMemoryStreamConfig(s.(*proxyman.SenderConfig).StreamSettings)
	if err != nil {
		t.Fatal(err)
	}
	return f.(*freedom.Config), m
}
