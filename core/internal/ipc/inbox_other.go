//go:build !unix

package ipc

import (
	"io"
	"net"
)

func inbox(c net.Conn) (io.Reader, func()) {
	return c, func() {}
}
