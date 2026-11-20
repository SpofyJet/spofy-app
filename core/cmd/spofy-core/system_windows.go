package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/SpofyJet/spofy-app/core/internal/desk"
	"github.com/SpofyJet/spofy-app/core/internal/store"
	"github.com/SpofyJet/spofy-app/core/internal/sub"
)

const (
	serviceName = "Spofy"
	pipe        = `\\.\pipe\spofy`
	logLimit    = 4 << 20
)

func system(name string, args []string) (int, bool) {
	switch name {
	case "service":
		return service(), true
	case "install":
		return report(install()), true
	case "uninstall":
		return report(uninstall()), true
	}
	return 0, false
}

func report(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func prepare() (served, func(), error) {
	root, err := desk.Root()
	if err != nil {
		return served{}, nil, err
	}
	if err := desk.Secure(root); err != nil {
		return served{}, nil, err
	}
	logs := io.Discard
	closer := func() {}
	file := filepath.Join(root, "service.log")
	if info, err := os.Stat(file); err == nil && info.Size() > logLimit {
		os.Rename(file, file+".1")
	}
	if f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		logs, closer = f, func() { f.Close() }
	}
	key, err := desk.Key(filepath.Join(root, "state.key"))
	if err != nil {
		closer()
		return served{}, nil, err
	}
	folder := filepath.Join(root, "state")
	if _, err := store.Open(folder, key); errors.Is(err, store.ErrUnreadable) {
		os.Rename(folder, fmt.Sprintf("%s.unreadable-%d", folder, time.Now().Unix()))
	}
	hwid, release, model := desk.Identity()
	return served{
		folder: folder, key: key, mode: "tun", level: "warning", guarded: true,
		device: sub.Device{HWID: hwid, OS: "Windows", OSVersion: release, Model: model, AppVersion: version},
		socket: pipe,
		watch:  time.Second, quiet: 5 * time.Second, renew: 15 * time.Minute, resample: time.Hour, settle: 10 * time.Second,
		logs: logs,
	}, closer, nil
}

type handler struct{}

func (handler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	o, closer, err := prepare()
	if err != nil {
		return true, 1
	}
	defer closer()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- o.run(ctx, io.Discard) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case c := <-requests:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending, WaitHint: 20000}
				cancel()
				select {
				case <-done:
				case <-time.After(20 * time.Second):
				}
				return false, 0
			}
		case err := <-done:
			if err != nil {
				fmt.Fprintln(o.logs, err)
				return true, 1
			}
			return false, 0
		}
	}
}

func service() int {
	if is, err := svc.IsWindowsService(); err != nil || !is {
		fmt.Fprintln(os.Stderr, "service runs only under the service manager, use install")
		return 2
	}
	if err := svc.Run(serviceName, handler{}); err != nil {
		return 1
	}
	return 0
}

func install() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	config := mgr.Config{
		DisplayName:  "Spofy",
		Description:  "Spofy VPN: туннель и защита от утечек",
		StartType:    mgr.StartAutomatic,
		ServiceType:  windows.SERVICE_WIN32_OWN_PROCESS,
		ErrorControl: mgr.ErrorNormal,
		SidType:      windows.SERVICE_SID_TYPE_UNRESTRICTED,
	}
	s, err := m.OpenService(serviceName)
	if err == nil {
		stop(s)
		config.BinaryPathName = fmt.Sprintf(`"%s" service`, exe)
		err = s.UpdateConfig(config)
	} else {
		s, err = m.CreateService(serviceName, exe, config, "service")
	}
	if err != nil {
		return err
	}
	defer s.Close()
	err = s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: time.Second},
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}, 24*60*60)
	if err != nil {
		return err
	}
	if err := s.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		return err
	}
	return s.Start()
}

func stop(s *mgr.Service) {
	q, err := s.Control(svc.Stop)
	for deadline := time.Now().Add(30 * time.Second); err == nil && q.State != svc.Stopped && time.Now().Before(deadline); {
		time.Sleep(200 * time.Millisecond)
		q, err = s.Query()
	}
}

func uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return nil
	}
	defer s.Close()
	stop(s)
	return s.Delete()
}
