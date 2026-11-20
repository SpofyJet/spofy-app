package guard

import (
	"errors"
	"net/netip"
	"os"
	"sync"

	"github.com/tailscale/wf"
	"golang.org/x/sys/windows"
)

var Kept = wf.SublayerID(windows.GUID{Data1: 0x5f0f5a7e, Data2: 0x1d2b, Data3: 0x4c3a, Data4: [8]byte{0x9b, 0x61, 0x73, 0x70, 0x6f, 0x66, 0x79, 0x01}})

type Windows struct {
	Local []netip.Addr

	mu    sync.Mutex
	s     *wf.Session
	keep  bool
	kept  bool
	leave bool
}

func (g *Windows) Held() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	s, err := wf.New(&wf.Options{Name: "Spofy"})
	if err != nil {
		return false
	}
	defer s.Close()
	g.kept = len(rules(s)) > 0
	return g.kept
}

func (g *Windows) Keep(on bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.keep = on
	if g.s == nil && !g.kept {
		return nil
	}
	if on {
		if err := g.hold(); err != nil {
			return err
		}
		g.drop()
		return nil
	}
	if err := g.open(); err != nil {
		return err
	}
	return g.release()
}

func (g *Windows) Raise() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.keep {
		if err := g.hold(); err != nil {
			return err
		}
		g.drop()
		return nil
	}
	if err := g.open(); err != nil {
		return err
	}
	if g.kept {
		return g.release()
	}
	return nil
}

func (g *Windows) Lower() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.drop()
	if g.leave {
		return nil
	}
	return g.release()
}

func (g *Windows) Leave() {
	g.mu.Lock()
	g.leave = true
	g.mu.Unlock()
}

func Release() error {
	g := &Windows{kept: true}
	return g.release()
}

func (g *Windows) open() error {
	if g.s != nil {
		return nil
	}
	app, err := self()
	if err != nil {
		return err
	}
	s, err := wf.New(&wf.Options{Name: "Spofy", Description: "Keeps traffic inside the tunnel", Dynamic: true})
	if err != nil {
		return err
	}
	id, err := windows.GenerateGUID()
	if err == nil {
		sub := wf.SublayerID(id)
		if err = s.AddSublayer(&wf.Sublayer{ID: sub, Name: "Spofy", Weight: 0xffff}); err == nil {
			err = g.fill(s, sub, app, false)
		}
	}
	if err != nil {
		s.Close()
		return err
	}
	g.s = s
	return nil
}

func (g *Windows) drop() {
	if g.s != nil {
		g.s.Close()
		g.s = nil
	}
}

func (g *Windows) hold() error {
	app, err := self()
	if err != nil {
		return err
	}
	s, err := wf.New(&wf.Options{Name: "Spofy"})
	if err != nil {
		return err
	}
	defer s.Close()
	old := rules(s)
	if len(old) == 0 {
		s.DeleteSublayer(Kept)
		if err := s.AddSublayer(&wf.Sublayer{ID: Kept, Name: "Spofy", Description: "Keeps traffic inside the tunnel while the service is down", Weight: 0xfffe, Persistent: true}); err != nil {
			return err
		}
	}
	if err := g.fill(s, Kept, app, true); err != nil {
		for _, r := range blocksFirst(s) {
			if !contains(old, r) {
				s.DeleteRule(r)
			}
		}
		if len(old) == 0 {
			s.DeleteSublayer(Kept)
		}
		return err
	}
	for _, r := range old {
		s.DeleteRule(r)
	}
	g.kept = true
	return nil
}

func (g *Windows) release() error {
	if !g.kept {
		return nil
	}
	s, err := wf.New(&wf.Options{Name: "Spofy"})
	if err != nil {
		return err
	}
	defer s.Close()
	var errs []error
	for _, r := range blocksFirst(s) {
		errs = append(errs, s.DeleteRule(r))
	}
	if err := s.DeleteSublayer(Kept); err != nil && !errors.Is(err, windows.Errno(0x80320007)) {
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	g.kept = false
	return nil
}

func rules(s *wf.Session) []wf.RuleID {
	all, err := s.Rules()
	if err != nil {
		return nil
	}
	var ours []wf.RuleID
	for _, r := range all {
		if r.Sublayer == Kept {
			ours = append(ours, r.ID)
		}
	}
	return ours
}

func blocksFirst(s *wf.Session) []wf.RuleID {
	all, err := s.Rules()
	if err != nil {
		return nil
	}
	var blocks, permits []wf.RuleID
	for _, r := range all {
		switch {
		case r.Sublayer != Kept:
		case r.Action == wf.ActionBlock:
			blocks = append(blocks, r.ID)
		default:
			permits = append(permits, r.ID)
		}
	}
	return append(blocks, permits...)
}

func contains(list []wf.RuleID, id wf.RuleID) bool {
	for _, r := range list {
		if r == id {
			return true
		}
	}
	return false
}

func self() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return wf.AppID(exe)
}

func (g *Windows) fill(s *wf.Session, sub wf.SublayerID, app string, persistent bool) error {
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
			if err := rule(s, layer.id, sub, wf.ActionPermit, 8, conditions, persistent); err != nil {
				return err
			}
		}
		if err := rule(s, layer.id, sub, wf.ActionBlock, 0, nil, persistent); err != nil {
			return err
		}
	}
	return nil
}

func rule(s *wf.Session, layer wf.LayerID, sub wf.SublayerID, action wf.Action, weight uint64, conditions []*wf.Match, persistent bool) error {
	id, err := windows.GenerateGUID()
	if err != nil {
		return err
	}
	return s.AddRule(&wf.Rule{ID: wf.RuleID(id), Name: "Spofy", Layer: layer, Sublayer: sub, Weight: weight, Conditions: conditions, Action: action, Persistent: persistent})
}
