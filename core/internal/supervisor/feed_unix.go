//go:build unix

package supervisor

import (
	"errors"
	"io"
	"net"
	"os"
	"os/exec"

	"github.com/SpofyJet/spofy-app/core/internal/host"
)

func feed(cmd *exec.Cmd, carries bool) (io.WriteCloser, func(), error) {
	if !carries {
		in, err := cmd.StdinPipe()
		return in, func() {}, err
	}
	near, far, err := host.Pair()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stdin = far
	return near, func() { far.Close() }, nil
}

func hand(in io.Writer, line []byte, f *os.File) error {
	c, ok := in.(*net.UnixConn)
	if !ok {
		return errors.New("the engine was started without a way to take a descriptor")
	}
	return host.Send(c, line, f)
}
