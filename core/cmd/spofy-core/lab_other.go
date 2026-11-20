//go:build !linux && !windows

package main

import (
	"os/exec"

	"github.com/SpofyJet/spofy-app/core/internal/supervisor"
	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
)

func labGuard(string) supervisor.Guard { return nil }

func device(string) supervisor.Guard { return nil }

func labIdentity(*exec.Cmd) {}

func desktop(*xcfg.Tun) {}
