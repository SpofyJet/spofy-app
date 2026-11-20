package main

import (
	"net/netip"
	"os/exec"

	"github.com/SpofyJet/spofy-app/core/internal/guard"
	"github.com/SpofyJet/spofy-app/core/internal/supervisor"
	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
)

func labGuard(string) supervisor.Guard {
	return &guard.Windows{Local: []netip.Addr{netip.MustParseAddr("172.19.0.1")}}
}

func labIdentity(*exec.Cmd) {}

func desktop(t *xcfg.Tun) {
	t.DNS, t.Resolver = []string{resolver}, []string{resolver}
}
