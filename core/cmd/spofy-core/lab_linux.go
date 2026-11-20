package main

import (
	"os/exec"
	"syscall"

	"github.com/SpofyJet/spofy-app/core/internal/guard"
	"github.com/SpofyJet/spofy-app/core/internal/supervisor"
	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
)

func labGuard(tun string) supervisor.Guard {
	return &guard.Linux{Tun: tun}
}

func labIdentity(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 0, Gid: guard.EngineGID}}
}

func desktop(*xcfg.Tun) {}
