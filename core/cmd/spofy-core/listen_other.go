//go:build !unix && !windows

package main

import (
	"context"
	"errors"

	"github.com/SpofyJet/spofy-app/core/internal/app"
	"github.com/SpofyJet/spofy-app/core/internal/host"
)

func listen(context.Context, string, *app.App, *host.Platform, func()) error {
	return errors.ErrUnsupported
}
