package guard

import (
	"net/netip"
	"os"
	"sync"

	"github.com/tailscale/wf"
	"golang.org/x/sys/windows"
)

type Windows struct {
	Local []netip.Addr

	mu sync.Mutex
	s  *wf.Session
}

func (g *Windows) Raise() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.s != nil {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	app, err := wf.AppID(exe)
	if err != nil {
		return err
	}
	s, err := wf.New(&wf.Options{Name: "Spofy", Description: "Keeps traffic inside the tunnel", Dynamic: true})
	if err != nil {
		return err
	}
	if err := g.fill(s, app); err != nil {
		s.Close()
		return err
	}
	g.s = s
	return nil
}

func (g *Windows) fill(s *wf.Session, app string) error {
	id, err := windows.GenerateGUID()
	if err != nil {
		return err
	}
	sub := wf.SublayerID(id)
	if err := s.AddSublayer(&wf.Sublayer{ID: sub, Name: "Spofy", Weight: 0xffff}); err != nil {
		return err
	}
	udp := func(local, remote uint16) []*wf.Match {
		return []*wf.Match{
			{Field: wf.FieldIPProtocol, Op: wf.MatchTypeEqual, Value: wf.IPProtoUDP},
			{Field: wf.FieldIPLocalPort, Op: wf.MatchTypeEqual, Value: local},
			{Field: wf.FieldIPRemotePort, Op: wf.MatchTypeEqual, Value: remote},
		}
	}
	for _, layer := range []struct {
		id  wf.LayerID
		six bool
	}{{wf.LayerALEAuthConnectV4, false}, {wf.LayerALEAuthConnectV6, true}} {
		allowed := [][]*wf.Match{
			{{Field: wf.FieldALEAppID, Op: wf.MatchTypeEqual, Value: app}},
			{{Field: wf.FieldFlags, Op: wf.MatchTypeFlagsAllSet, Value: wf.ConditionFlagIsLoopback}},
		}
		for _, a := range g.Local {
			if a.Is6() == layer.six {
				allowed = append(allowed, []*wf.Match{{Field: wf.FieldIPLocalAddress, Op: wf.MatchTypeEqual, Value: a}})
			}
		}
		if layer.six {
			allowed = append(allowed, udp(546, 547))
			for kind := uint16(133); kind <= 137; kind++ {
				allowed = append(allowed, []*wf.Match{
					{Field: wf.FieldIPProtocol, Op: wf.MatchTypeEqual, Value: wf.IPProtoICMPV6},
					{Field: wf.FieldIPLocalPort, Op: wf.MatchTypeEqual, Value: kind},
				})
			}
		} else {
			allowed = append(allowed, udp(68, 67))
		}
		for _, conditions := range allowed {
			if err := rule(s, layer.id, sub, wf.ActionPermit, 8, conditions); err != nil {
				return err
			}
		}
		if err := rule(s, layer.id, sub, wf.ActionBlock, 0, nil); err != nil {
			return err
		}
	}
	return nil
}

func rule(s *wf.Session, layer wf.LayerID, sub wf.SublayerID, action wf.Action, weight uint64, conditions []*wf.Match) error {
	id, err := windows.GenerateGUID()
	if err != nil {
		return err
	}
	return s.AddRule(&wf.Rule{ID: wf.RuleID(id), Name: "Spofy", Layer: layer, Sublayer: sub, Weight: weight, Conditions: conditions, Action: action})
}

func (g *Windows) Lower() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.s == nil {
		return nil
	}
	err := g.s.Close()
	g.s = nil
	return err
}
