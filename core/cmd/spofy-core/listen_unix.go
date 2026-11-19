//go:build unix

package main

import (
	"context"
	"net"
	"os"
	"syscall"

	"github.com/SpofyJet/spofy-app/core/internal/app"
	"github.com/SpofyJet/spofy-app/core/internal/host"
	"github.com/SpofyJet/spofy-app/core/internal/ipc"
)

func listen(ctx context.Context, path string, a *app.App, p *host.Platform, ready func()) error {
	os.Remove(path)
	mask := syscall.Umask(0o077)
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path})
	syscall.Umask(mask)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	ready()
	return ipc.Listen(ctx, l, a, p)
}
