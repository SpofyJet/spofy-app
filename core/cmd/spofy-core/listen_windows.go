package main

import (
	"context"

	"github.com/SpofyJet/spofy-app/core/internal/app"
	"github.com/SpofyJet/spofy-app/core/internal/host"
	"github.com/SpofyJet/spofy-app/core/internal/ipc"
)

func listen(ctx context.Context, path string, a *app.App, p *host.Platform, ready func()) error {
	l, err := ipc.ListenPipe(path)
	if err != nil {
		return err
	}
	ready()
	return ipc.Listen(ctx, l, a, p)
}
