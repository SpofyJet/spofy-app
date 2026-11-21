package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/SpofyJet/spofy-app/core/internal/desk"
	"github.com/SpofyJet/spofy-app/core/internal/guard"
)

func (s *setup) handOff(self string) int {
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("spofy-remove-%d", time.Now().UnixNano()))
	if err := desk.Secure(dir); err != nil {
		return s.fail(codeFailed, says().removeFailed, err)
	}
	if s.logPath == "" {
		defer s.open(filepath.Join(dir, "uninstall.log"))()
	}
	copied := filepath.Join(dir, uninstaller)
	if err := copyFile(self, copied); err != nil {
		later(dir)
		return s.fail(codeFailed, says().removeFailed, err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		later(dir)
		return s.fail(codeFailed, says().removeFailed, err)
	}
	defer r.Close()
	if err := windows.SetHandleInformation(windows.Handle(w.Fd()), windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
		w.Close()
		later(dir)
		return s.fail(codeFailed, says().removeFailed, err)
	}
	args := []string{"-from", s.home, "-report", strconv.FormatUint(uint64(w.Fd()), 10)}
	if s.logPath != "" {
		args = append(args, "-log", s.logPath)
	}
	if s.quiet {
		args = append(args, "-quiet")
	}
	cmd := exec.Command(copied, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{AdditionalInheritedHandles: []syscall.Handle{syscall.Handle(w.Fd())}}
	err = cmd.Start()
	w.Close()
	if err != nil {
		later(dir)
		return s.fail(codeFailed, says().removeFailed, err)
	}
	b, _ := io.ReadAll(r)
	code, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		s.printf("the copy that removes Spofy gave no result: %q", b)
		return codeFailed
	}
	return code
}

func (s *setup) removeFrom(from, self string, report uintptr) int {
	if !strings.EqualFold(filepath.Clean(from), s.home) {
		return s.fail(codeUsage, says().removeFailed, fmt.Errorf("%s is not the folder of Spofy", from))
	}
	parent, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(os.Getppid()))
	if err != nil {
		parent = 0
	}
	code := s.remove()
	if report != 0 {
		f := os.NewFile(report, "report")
		fmt.Fprint(f, code)
		f.Close()
	}
	if parent != 0 {
		windows.WaitForSingleObject(parent, 30000)
		windows.CloseHandle(parent)
	}
	if code == codeOK {
		var err error
		for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(100 * time.Millisecond) {
			if err = os.RemoveAll(s.home); err == nil || time.Now().After(deadline) {
				break
			}
		}
		if err != nil {
			s.printf("the folder goes at the next restart: %v", err)
			later(s.home)
		}
	}
	later(filepath.Dir(self))
	return code
}

func (s *setup) remove() int {
	sa := says()
	release, ok := single()
	if !ok {
		return s.fail(codeBusy, sa.busy, errors.New("another setup runs"))
	}
	defer release()
	if !s.quiet && !confirm(sa.ask, sa.askMore) {
		s.printf("the person kept Spofy")
		return codeCancelled
	}
	p := &progress{}
	var o outcome
	p.run(s.quiet, sa.removing, func() { o = s.takeOut(p) })
	s.printf("removal finished with %d", o.code)
	if !s.quiet {
		if o.code == codeOK {
			tell(sa.removed, "", false)
		} else if o.main != "" {
			tell(o.main, o.more, true)
		}
	}
	return o.code
}

func (s *setup) takeOut(p *progress) outcome {
	sa := says()
	ran := s.closeApps(5*time.Second, window, tray)
	p.permille.Store(200)
	if exists(filepath.Join(s.home, core)) {
		s.greet(ran)
		if err := s.core("uninstall"); err != nil {
			s.printf("the service did not let go: %v", err)
			return outcome{codeFailed, sa.removeFailed, sa.removeFailedMore}
		}
	} else if err := s.orphan(); err != nil {
		s.printf("what the service left: %v", err)
		return outcome{codeFailed, sa.removeFailed, sa.removeFailedMore}
	}
	p.permille.Store(700)
	s.closeApps(0, core, window, tray)
	if err := registry.DeleteKey(registry.LOCAL_MACHINE, entryKey); err != nil && !errors.Is(err, registry.ErrNotExist) {
		s.printf("entry: %v", err)
	}
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, runKey, registry.SET_VALUE); err == nil {
		k.DeleteValue("Spofy")
		k.Close()
	}
	if programs, err := windows.KnownFolderPath(windows.FOLDERID_CommonPrograms, 0); err == nil {
		os.Remove(filepath.Join(programs, "Spofy.lnk"))
	}
	entries, _ := os.ReadDir(s.home)
	for _, e := range entries {
		if strings.EqualFold(e.Name(), uninstaller) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(s.home, e.Name())); err != nil {
			s.printf("%s goes at the next restart: %v", e.Name(), err)
			later(filepath.Join(s.home, e.Name()))
		}
	}
	p.permille.Store(1000)
	return outcome{codeOK, "", ""}
}

func (s *setup) orphan() error {
	stopService()
	s.forget()
	err := guard.Release()
	if root, rerr := desk.Root(); rerr == nil {
		err = errors.Join(err, desk.Forget(root))
	}
	return err
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
