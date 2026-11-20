//go:build unix

package ipc

import (
	"io"
	"net"

	"github.com/SpofyJet/spofy-app/core/internal/host"
)

func inbox(c net.Conn) (io.Reader, func()) {
	u, ok := c.(*net.UnixConn)
	if !ok {
		return c, func() {}
	}
	in := host.NewInbox(u)
	return in, in.Drop
}
