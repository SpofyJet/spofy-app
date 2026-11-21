package desk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"time"
)

const (
	invitation = "launch.json"
	fresh      = 2 * time.Minute
)

var Invitable = []string{"spofy-tray.exe", "spofy.exe"}

type Run struct {
	Session uint32 `json:"session"`
	Program string `json:"program"`
}

type launch struct {
	At   time.Time `json:"at"`
	Runs []Run     `json:"runs"`
}

func Invite(root string, runs []Run, now time.Time) error {
	b, err := json.Marshal(launch{At: now.UTC(), Runs: runs})
	if err != nil {
		return err
	}
	path := filepath.Join(root, invitation)
	tmp := path + ".new"
	os.Remove(tmp)
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func Invited(root string, now time.Time) []Run {
	path := filepath.Join(root, invitation)
	b, err := os.ReadFile(path)
	os.Remove(path)
	if err != nil {
		return nil
	}
	var l launch
	if json.Unmarshal(b, &l) != nil || now.Sub(l.At) > fresh || l.At.Sub(now) > time.Minute {
		return nil
	}
	var runs []Run
	for _, r := range l.Runs {
		if r.Session != 0 && slices.Contains(Invitable, r.Program) && !slices.Contains(runs, r) {
			runs = append(runs, r)
		}
	}
	return runs
}

func Uninvite(root string) {
	os.Remove(filepath.Join(root, invitation))
}
