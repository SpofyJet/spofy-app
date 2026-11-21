package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"

	"github.com/SpofyJet/spofy-app/core/internal/desk"
	"github.com/SpofyJet/spofy-app/core/internal/pack"
)

const (
	codeOK         = 0
	codeFailed     = 1
	codeUsage      = 2
	codeBusy       = 3
	codeRolledBack = 9
	codeCancelled  = 1223
	uninstaller    = "uninstall.exe"
	logLimit       = 1 << 20
)

type setup struct {
	quiet   bool
	home    string
	session uint32
	logPath string
	log     *log.Logger
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("spofy-setup", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	quiet := fs.Bool("quiet", false, "")
	remove := fs.Bool("uninstall", false, "")
	from := fs.String("from", "", "")
	report := fs.Uint64("report", 0, "")
	logTo := fs.String("log", "", "")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return codeUsage
	}
	s := &setup{quiet: *quiet, log: log.New(io.Discard, "", 0)}
	defer s.open(*logTo)()
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &s.session); err != nil {
		s.session = 0
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return s.fail(codeFailed, says().admin, errors.New("not elevated"))
	}
	pf, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return s.fail(codeFailed, says().failed, err)
	}
	s.home = filepath.Join(pf, "Spofy")
	self, err := os.Executable()
	if err != nil {
		return s.fail(codeFailed, says().failed, err)
	}
	if *from != "" {
		return s.removeFrom(*from, self, uintptr(*report))
	}
	inside := strings.EqualFold(filepath.Clean(self), filepath.Join(s.home, uninstaller))
	if *remove || inside {
		return s.handOff(self)
	}
	if s.logPath == "" {
		if root, err := desk.Root(); err == nil && desk.Secure(root) == nil {
			defer s.open(filepath.Join(root, "setup.log"))()
		}
	}
	a, f, err := archive(self)
	if err != nil {
		return s.fail(codeFailed, says().damaged, err)
	}
	defer f.Close()
	release, ok := single()
	if !ok {
		return s.fail(codeBusy, says().busy, errors.New("another setup runs"))
	}
	defer release()
	return s.install(a)
}

func (s *setup) open(path string) func() {
	if path == "" {
		return func() {}
	}
	if info, err := os.Stat(path); err == nil && info.Size() > logLimit {
		os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return func() {}
	}
	s.logPath = path
	s.log = log.New(f, "", log.LstdFlags|log.Lmicroseconds)
	s.log.Printf("spofy setup %s, args %q", version, os.Args[1:])
	return func() { f.Close() }
}

func (s *setup) fail(code int, text string, err error) int {
	s.log.Printf("failed (%d): %v", code, err)
	if !s.quiet {
		tell(says().title, text, true)
	}
	return code
}

func archive(self string) (*pack.Archive, *os.File, error) {
	f, err := os.Open(self)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	a, err := pack.Open(f, info.Size())
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return a, f, nil
}

func single() (func(), bool) {
	name, _ := windows.UTF16PtrFromString(`Global\SpofySetup`)
	h, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		if h != 0 {
			windows.CloseHandle(h)
		}
		return nil, false
	}
	return func() { windows.CloseHandle(h) }, true
}

var version = "dev"

func (s *setup) printf(format string, args ...any) {
	s.log.Output(2, fmt.Sprintf(format, args...))
}
