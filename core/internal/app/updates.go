package app

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/engine"
	"github.com/SpofyJet/spofy-app/core/internal/update"
)

const (
	EvUpdate = "update"

	CodeUpdateOff         engine.Code = "update_off"
	CodeUpdateNone        engine.Code = "update_none"
	CodeUpdateUnreachable engine.Code = "update_unreachable"
	CodeUpdateBroken      engine.Code = "update_broken"
	CodeUpdateFile        engine.Code = "update_file"
	CodeUpdateBusy        engine.Code = "update_busy"
)

type Updates struct {
	Sources  []string
	Keys     []ed25519.PublicKey
	Platform string
	Dir      string
	First    time.Duration
	Every    time.Duration
}

type Update struct {
	Version  string `json:"version"`
	Build    int    `json:"build"`
	Notes    string `json:"notes"`
	Required bool   `json:"required"`
	Size     int64  `json:"size"`
	Done     int64  `json:"done"`
	File     string `json:"file"`
}

func (a *App) updatesOn() bool {
	u := a.cfg.Updates
	return len(u.Sources) > 0 && len(u.Keys) > 0 && u.Platform != "" && u.Dir != "" && a.cfg.Build > 0
}

func (a *App) watchUpdates() {
	u := a.cfg.Updates
	wait := u.First
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = u.Every
		if a.online.Load() {
			ctx, cancel := context.WithTimeout(a.ctx, time.Minute)
			a.CheckUpdate(ctx)
			cancel()
		}
		if wait <= 0 {
			return
		}
	}
}

func (a *App) CheckUpdate(ctx context.Context) error {
	if !a.updatesOn() {
		return fail(CodeUpdateOff, "updates are off in this build")
	}
	u := a.cfg.Updates
	offer, err := update.Check(ctx, a.cfg.HTTP, u.Sources, u.Keys, a.cfg.Build, u.Platform, a.cfg.Device.HWID)
	if err != nil {
		code := CodeUpdateUnreachable
		if errors.Is(err, update.ErrSignature) || errors.Is(err, update.ErrManifest) {
			code = CodeUpdateBroken
		}
		a.Note("app", "update check failed: "+string(code))
		return fail(code, "no answer about updates")
	}
	keep := ""
	a.umu.Lock()
	switch {
	case offer == nil:
		a.offer, a.upd = nil, nil
	case a.upd != nil && a.upd.Build == offer.Build:
		a.offer = offer
		keep = offer.File.Name
	default:
		a.offer = offer
		a.upd = &Update{Version: offer.Version, Build: offer.Build, Notes: offer.Notes, Required: offer.Required, Size: offer.File.Size}
		keep = offer.File.Name
	}
	now := a.updateNow()
	a.umu.Unlock()
	sweepUpdates(u.Dir, keep)
	a.emit(Event{Ev: EvUpdate, Update: now})
	if offer == nil {
		return fail(CodeUpdateNone, "this is the latest build")
	}
	a.Note("app", "update "+offer.Version+" is offered")
	return nil
}

func (a *App) FetchUpdate() error {
	a.umu.Lock()
	offer, busy := a.offer, a.fetching
	if offer != nil && !busy {
		a.fetching = true
	}
	a.umu.Unlock()
	switch {
	case offer == nil:
		return fail(CodeUpdateNone, "no update is offered")
	case busy:
		return fail(CodeUpdateBusy, "the update is already being downloaded")
	}
	defer func() {
		a.umu.Lock()
		a.fetching = false
		a.umu.Unlock()
	}()
	var shown int64 = -1
	progress := func(done, total int64) {
		if total <= 0 || done >= total || (shown >= 0 && (done-shown)*100 < total) {
			return
		}
		shown = done
		a.umu.Lock()
		if a.upd == nil || a.upd.Build != offer.Build {
			a.umu.Unlock()
			return
		}
		a.upd.Done = done
		now := a.updateNow()
		a.umu.Unlock()
		a.emit(Event{Ev: EvUpdate, Update: now})
	}
	path, err := update.Download(a.ctx, a.cfg.HTTP, offer.File, a.cfg.Updates.Dir, progress)
	if err != nil {
		a.Note("app", "update download failed")
		return fail(CodeUpdateFile, "the update was not downloaded")
	}
	a.umu.Lock()
	if a.upd != nil && a.upd.Build == offer.Build {
		a.upd.Done, a.upd.File = offer.File.Size, path
	}
	now := a.updateNow()
	a.umu.Unlock()
	a.emit(Event{Ev: EvUpdate, Update: now})
	a.Note("app", "update "+offer.Version+" is downloaded and checked")
	return nil
}

func (a *App) updateNow() *Update {
	if a.upd == nil {
		return nil
	}
	copied := *a.upd
	return &copied
}

func sweepUpdates(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.Type().IsRegular() && e.Name() != keep && !strings.HasPrefix(e.Name(), ".") {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func (a *App) InstallUpdate() error {
	a.umu.Lock()
	offer := a.offer
	file := ""
	if a.upd != nil {
		file = a.upd.File
	}
	a.umu.Unlock()
	switch {
	case a.cfg.Install == nil:
		return fail(CodeUpdateOff, "this system installs the update itself")
	case offer == nil || file == "":
		return fail(CodeUpdateNone, "no downloaded update")
	}
	if !intact(file, offer.File) {
		a.umu.Lock()
		if a.upd != nil {
			a.upd.Done, a.upd.File = 0, ""
		}
		now := a.updateNow()
		a.umu.Unlock()
		os.Remove(file)
		a.emit(Event{Ev: EvUpdate, Update: now})
		return fail(CodeUpdateFile, "the downloaded update changed")
	}
	a.Note("app", "update "+offer.Version+" is being installed")
	if err := a.cfg.Install(file); err != nil {
		return fail(CodeUpdateFile, "the installer did not start")
	}
	return nil
}

func intact(path string, f update.File) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	h := sha256.New()
	n, err := io.Copy(h, file)
	return err == nil && n == f.Size && hex.EncodeToString(h.Sum(nil)) == f.SHA256
}
