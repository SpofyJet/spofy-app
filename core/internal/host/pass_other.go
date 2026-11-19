//go:build !unix

package host

import (
	"errors"
	"io"
	"net"
	"os"
)

func Pair() (*net.UnixConn, *os.File, error) {
	return nil, nil, errors.ErrUnsupported
}

func Send(*net.UnixConn, []byte, *os.File) error {
	return errors.ErrUnsupported
}

func Stdin() io.Reader {
	return os.Stdin
}
