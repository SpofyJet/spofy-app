package sysproxy

import (
	"errors"
	"regexp"
	"slices"
	"strings"
)

const (
	Direct uint32 = 1
	Manual uint32 = 2
	Script uint32 = 4
	Detect uint32 = 8
)

var Bypass = strings.Join([]string{
	"localhost", "127.*", "10.*",
	"172.16.*", "172.17.*", "172.18.*", "172.19.*", "172.20.*", "172.21.*", "172.22.*", "172.23.*",
	"172.24.*", "172.25.*", "172.26.*", "172.27.*", "172.28.*", "172.29.*", "172.30.*", "172.31.*",
	"192.168.*", "169.254.*", "[::1]", "*.local", "<local>",
}, ";")

type Settings struct {
	Flags  uint32 `json:"flags"`
	Server string `json:"server,omitempty"`
	Bypass string `json:"bypass,omitempty"`
	Script string `json:"script,omitempty"`
}

type Record struct {
	Applied  string              `json:"applied"`
	Previous map[string]Settings `json:"previous"`
}

type System interface {
	Connections() ([]string, error)
	Get(conn string) (Settings, error)
	Put(conn string, s Settings) error
	Notify() error
	Load() (*Record, error)
	Save(*Record) error
	Forget() error
}

func Ours(address string) Settings {
	return Settings{Flags: Direct | Manual, Server: address, Bypass: Bypass}
}

var loopback = regexp.MustCompile(`^127\.0\.0\.1:[0-9]{1,5}$`)

func leftover(s Settings) bool {
	return loopback.MatchString(s.Server) && s.Bypass == Bypass
}

func On(sys System, address string) error {
	rec, err := sys.Load()
	if err != nil {
		return err
	}
	conns, err := sys.Connections()
	if err != nil {
		return err
	}
	next := &Record{Applied: address, Previous: map[string]Settings{}}
	var taken []string
	for _, c := range conns {
		now, err := sys.Get(c)
		if err != nil && c == "" {
			return err
		}
		if err != nil {
			continue
		}
		taken = append(taken, c)
		prev, kept := Settings{}, false
		if rec != nil {
			prev, kept = rec.Previous[c]
			kept = kept && now.Server == rec.Applied
		}
		switch {
		case kept:
			next.Previous[c] = prev
		case leftover(now):
			next.Previous[c] = Settings{Flags: Direct}
		default:
			next.Previous[c] = now
		}
	}
	if err := sys.Save(next); err != nil {
		return err
	}
	var errs []error
	for _, c := range taken {
		errs = append(errs, sys.Put(c, Ours(address)))
	}
	return errors.Join(append(errs, sys.Notify())...)
}

func Off(sys System) error {
	rec, err := sys.Load()
	if err != nil || rec == nil {
		return err
	}
	conns, err := sys.Connections()
	if err != nil {
		return err
	}
	left := &Record{Applied: rec.Applied, Previous: map[string]Settings{}}
	var errs []error
	names := make([]string, 0, len(rec.Previous))
	for c := range rec.Previous {
		names = append(names, c)
	}
	slices.Sort(names)
	for _, c := range names {
		if !slices.Contains(conns, c) {
			continue
		}
		prev := rec.Previous[c]
		now, err := sys.Get(c)
		if err == nil && now.Server != rec.Applied {
			continue
		}
		if err == nil {
			err = sys.Put(c, prev)
		}
		if err != nil {
			errs = append(errs, err)
			left.Previous[c] = prev
		}
	}
	if len(left.Previous) > 0 {
		errs = append(errs, sys.Save(left))
	} else {
		errs = append(errs, sys.Forget())
	}
	return errors.Join(append(errs, sys.Notify())...)
}
