package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/SpofyJet/spofy-app/core/internal/desk"
	"github.com/SpofyJet/spofy-app/core/internal/pack"
)

const (
	entryKey    = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Spofy`
	runKey      = `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
	desktopMark = "DesktopShortcut"
	window      = "spofy.exe"
	tray        = "spofy-tray.exe"
	core        = "spofy-core.exe"
	unpacked    = 850
)

var errCancelled = errors.New("cancelled by the person")

type outcome struct {
	code       int
	main, more string
}

func (s *setup) install(a *pack.Archive) int {
	p := &progress{}
	p.cancellable.Store(true)
	var o outcome
	p.run(s.quiet, says().installing, func() { o = s.put(a, p) })
	s.printf("finished with %d", o.code)
	if o.code != codeOK && o.main != "" && !s.quiet {
		tell(o.main, o.more, true)
	}
	return o.code
}

func (s *setup) put(a *pack.Archive, p *progress) outcome {
	sa := says()
	s.printf("installing %s into %s: %d bytes in %d", a.Version, s.home, a.Unpacked, a.Size)
	if err := a.Verify(); err != nil {
		s.printf("verify: %v", err)
		return outcome{codeFailed, sa.damaged, ""}
	}
	s.recover()
	fresh := fmt.Sprintf("%s.new-%d", s.home, time.Now().UnixNano())
	began := time.Now()
	err := a.Extract(fresh, func(done, total int64) error {
		p.permille.Store(done * unpacked / total)
		if p.cancelled.Load() {
			return errCancelled
		}
		return nil
	})
	p.cancellable.Store(false)
	if err == nil && p.cancelled.Load() {
		err = errCancelled
	}
	if err != nil {
		s.printf("unpack: %v", err)
		os.RemoveAll(fresh)
		switch {
		case errors.Is(err, errCancelled):
			return outcome{codeCancelled, "", ""}
		case errors.Is(err, pack.ErrDamaged):
			return outcome{codeFailed, sa.damaged, ""}
		}
		return outcome{codeFailed, sa.failed, err.Error()}
	}
	s.printf("unpacked in %v", time.Since(began))
	before := s.entry()
	upgrade := exists(filepath.Join(s.home, core))
	var ran []desk.Run
	if upgrade {
		ran = s.closeApps(5*time.Second, window, tray)
		if _, err := stopService(); err != nil {
			s.printf("stop: %v", err)
			s.greet(ran)
			s.startService()
			os.RemoveAll(fresh)
			return outcome{codeFailed, sa.inUse, ""}
		}
		s.closeApps(0, core)
	}
	p.permille.Store(unpacked + 30)
	old := ""
	if exists(s.home) {
		old = fmt.Sprintf("%s.old-%d", s.home, time.Now().UnixNano())
		if err := rename(s.home, old); err != nil {
			s.printf("set the running version aside: %v", err)
			s.greet(ran)
			s.startService()
			os.RemoveAll(fresh)
			return outcome{codeFailed, sa.inUse, ""}
		}
	}
	if err := rename(fresh, s.home); err != nil {
		s.printf("put the new version in place: %v", err)
		if old != "" {
			rename(old, s.home)
		}
		s.greet(ran)
		s.startService()
		os.RemoveAll(fresh)
		return outcome{codeFailed, sa.failed, err.Error()}
	}
	p.permille.Store(unpacked + 60)
	s.greet(s.plan(ran, upgrade))
	if err := s.core("install"); err != nil {
		s.printf("the new version did not start: %v", err)
		return s.rollback(old, ran, before)
	}
	p.permille.Store(unpacked + 120)
	if err := s.register(a); err != nil {
		s.printf("register: %v", err)
		return s.rollback(old, ran, before)
	}
	if old != "" {
		if err := os.RemoveAll(old); err != nil {
			s.printf("the old version stays until a restart: %v", err)
			later(old)
		}
	}
	p.permille.Store(1000)
	s.printf("installed %s in %v", a.Version, time.Since(began))
	return outcome{codeOK, "", ""}
}

func (s *setup) rollback(old string, ran []desk.Run, before map[string]any) outcome {
	sa := says()
	if _, err := stopService(); err != nil {
		s.printf("stop the new version: %v", err)
	}
	s.closeApps(0, core, window, tray)
	bad := fmt.Sprintf("%s.bad-%d", s.home, time.Now().UnixNano())
	if err := rename(s.home, bad); err != nil {
		s.printf("set the new version aside: %v", err)
		return outcome{codeFailed, sa.failed, err.Error()}
	}
	if old == "" {
		if err := s.orphan(); err != nil {
			s.printf("what the version that did not start left: %v", err)
		}
		os.RemoveAll(bad)
		return outcome{codeFailed, sa.failed, ""}
	}
	if err := rename(old, s.home); err != nil {
		s.printf("put the running version back: %v", err)
		return outcome{codeFailed, sa.failed, err.Error()}
	}
	s.greet(ran)
	if err := s.core("install"); err != nil {
		s.printf("the running version did not start again: %v", err)
	}
	s.restore(before)
	if err := os.RemoveAll(bad); err != nil {
		later(bad)
	}
	return outcome{codeRolledBack, sa.rolledBack, sa.rolledBackMore}
}

func (s *setup) recover() {
	leftovers, _ := filepath.Glob(s.home + ".*-*")
	sort.Strings(leftovers)
	if !exists(s.home) {
		for i := len(leftovers) - 1; i >= 0; i-- {
			if strings.HasPrefix(leftovers[i], s.home+".old-") && rename(leftovers[i], s.home) == nil {
				s.printf("put back %s cut in an earlier setup", leftovers[i])
				leftovers = slices.Delete(leftovers, i, i+1)
				break
			}
		}
	}
	for _, l := range leftovers {
		for _, kind := range []string{".new-", ".old-", ".bad-"} {
			if strings.HasPrefix(l, s.home+kind) {
				s.printf("remove %s left by an earlier setup: %v", l, os.RemoveAll(l))
			}
		}
	}
}

func (s *setup) plan(ran []desk.Run, upgrade bool) []desk.Run {
	runs := slices.Clone(ran)
	add := func(program string) {
		if s.session != 0 && !slices.Contains(runs, desk.Run{Session: s.session, Program: program}) {
			runs = append(runs, desk.Run{Session: s.session, Program: program})
		}
	}
	if !upgrade && exists(filepath.Join(s.home, tray)) {
		add(tray)
	}
	if !s.quiet {
		add(window)
	}
	return runs
}

func (s *setup) greet(runs []desk.Run) {
	if len(runs) == 0 {
		return
	}
	root, err := desk.Root()
	if err == nil {
		err = desk.Secure(root)
	}
	if err != nil {
		s.printf("greet: %v", err)
		return
	}
	if err := desk.Invite(root, runs, time.Now()); err != nil {
		s.printf("greet: %v", err)
		return
	}
	s.printf("the service will start %+v", runs)
}

func (s *setup) core(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(s.home, core), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	out, err := cmd.CombinedOutput()
	s.printf("%s %v: %v %s", core, args, err, strings.TrimSpace(string(out)))
	return err
}

func (s *setup) startService() {
	m, err := mgr.Connect()
	if err != nil {
		return
	}
	defer m.Disconnect()
	if sv, err := m.OpenService(serviceName); err == nil {
		s.printf("start the running version again: %v", sv.Start())
		sv.Close()
	}
}

func (s *setup) forget() {
	m, err := mgr.Connect()
	if err != nil {
		return
	}
	defer m.Disconnect()
	if sv, err := m.OpenService(serviceName); err == nil {
		s.printf("remove the service of a version that did not start: %v", sv.Delete())
		sv.Close()
	}
	if root, err := desk.Root(); err == nil {
		desk.Uninvite(root)
		os.Remove(root)
	}
}

func (s *setup) entry() map[string]any {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, entryKey, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	out := map[string]any{}
	if v, _, err := k.GetStringValue("DisplayVersion"); err == nil {
		out["DisplayVersion"] = v
	}
	if v, _, err := k.GetIntegerValue("EstimatedSize"); err == nil {
		out["EstimatedSize"] = uint32(v)
	}
	return out
}

func (s *setup) restore(before map[string]any) {
	if before == nil {
		return
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, entryKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if v, ok := before["DisplayVersion"].(string); ok {
		k.SetStringValue("DisplayVersion", v)
	}
	if v, ok := before["EstimatedSize"].(uint32); ok {
		k.SetDWordValue("EstimatedSize", v)
	}
}

func (s *setup) register(a *pack.Archive) error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, entryKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	remove := windows.EscapeArg(filepath.Join(s.home, uninstaller)) + " -uninstall"
	for name, value := range map[string]string{
		"DisplayName":          "Spofy",
		"DisplayVersion":       a.Version,
		"Publisher":            "Spofy",
		"DisplayIcon":          filepath.Join(s.home, window) + ",0",
		"InstallLocation":      s.home,
		"UninstallString":      remove,
		"QuietUninstallString": remove + " -quiet",
	} {
		if err := k.SetStringValue(name, value); err != nil {
			return err
		}
	}
	for name, value := range map[string]uint32{"NoModify": 1, "NoRepair": 1, "EstimatedSize": uint32((a.Unpacked + 1023) / 1024)} {
		if err := k.SetDWordValue(name, value); err != nil {
			return err
		}
	}
	if exists(filepath.Join(s.home, tray)) {
		r, _, err := registry.CreateKey(registry.LOCAL_MACHINE, runKey, registry.SET_VALUE)
		if err != nil {
			return err
		}
		err = r.SetStringValue("Spofy", windows.EscapeArg(filepath.Join(s.home, tray)))
		r.Close()
		if err != nil {
			return err
		}
	}
	programs, err := windows.KnownFolderPath(windows.FOLDERID_CommonPrograms, 0)
	if err != nil {
		return err
	}
	if err := shortcut(filepath.Join(programs, "Spofy.lnk"), filepath.Join(s.home, window), "Spofy VPN"); err != nil {
		return err
	}
	if placed, _, err := k.GetIntegerValue(desktopMark); err == nil && placed != 0 {
		return nil
	}
	desktop, err := windows.KnownFolderPath(windows.FOLDERID_PublicDesktop, 0)
	if err == nil {
		err = shortcut(filepath.Join(desktop, "Spofy.lnk"), filepath.Join(s.home, window), "Spofy VPN")
	}
	if err == nil {
		err = k.SetDWordValue(desktopMark, 1)
	}
	s.printf("a shortcut on the desktop: %v", err)
	return nil
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func rename(from, to string) error {
	var err error
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		if err = os.Rename(from, to); err == nil || time.Now().After(deadline) {
			return err
		}
	}
}

func later(path string) {
	filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			windows.MoveFileEx(windows.StringToUTF16Ptr(p), nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
		}
		return nil
	})
	var dirs []string
	filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, p)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		windows.MoveFileEx(windows.StringToUTF16Ptr(dirs[i]), nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
	}
}
