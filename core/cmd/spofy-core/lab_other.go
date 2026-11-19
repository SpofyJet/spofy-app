//go:build !linux

package main

import (
	"os/exec"

	"github.com/SpofyJet/spofy-app/core/internal/supervisor"
)

func labGuard(string) supervisor.Guard { return nil }

func labIdentity(*exec.Cmd) {}
