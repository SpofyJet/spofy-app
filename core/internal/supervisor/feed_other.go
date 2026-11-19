//go:build !unix

package supervisor

import (
	"errors"
	"io"
	"os"
	"os/exec"
)

func feed(cmd *exec.Cmd, carries bool) (io.WriteCloser, func(), error) {
	if carries {
		return nil, nil, errors.ErrUnsupported
	}
	in, err := cmd.StdinPipe()
	return in, func() {}, err
}

func hand(io.Writer, []byte, *os.File) error {
	return errors.ErrUnsupported
}
