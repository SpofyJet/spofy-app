//go:build unix

package host

import (
	"errors"
	"io"
	"net"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

func Pair() (*net.UnixConn, *os.File, error) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	near := os.NewFile(uintptr(fds[0]), "near")
	far := os.NewFile(uintptr(fds[1]), "far")
	conn, err := net.FileConn(near)
	near.Close()
	if err != nil {
		far.Close()
		return nil, nil, err
	}
	return conn.(*net.UnixConn), far, nil
}

func Send(c *net.UnixConn, line []byte, f *os.File) error {
	raw, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var sent error
	err = raw.Control(func(fd uintptr) {
		_, _, sent = c.WriteMsgUnix(line, unix.UnixRights(int(fd)), nil)
	})
	return errors.Join(err, sent)
}

type Inbox struct {
	conn *net.UnixConn
	oob  []byte

	mu  sync.Mutex
	fds []int
}

func NewInbox(c *net.UnixConn) *Inbox {
	return &Inbox{conn: c, oob: make([]byte, unix.CmsgSpace(4*4))}
}

func Stdin() io.Reader {
	if st, err := os.Stdin.Stat(); err != nil || st.Mode()&os.ModeSocket == 0 {
		return os.Stdin
	}
	c, err := net.FileConn(os.Stdin)
	if err != nil {
		return os.Stdin
	}
	u, ok := c.(*net.UnixConn)
	if !ok {
		c.Close()
		return os.Stdin
	}
	return NewInbox(u)
}

func (i *Inbox) Read(p []byte) (int, error) {
	n, oobn, _, _, err := i.conn.ReadMsgUnix(p, i.oob)
	if oobn > 0 {
		msgs, _ := unix.ParseSocketControlMessage(i.oob[:oobn])
		for _, m := range msgs {
			fds, _ := unix.ParseUnixRights(&m)
			for _, fd := range fds {
				unix.CloseOnExec(fd)
			}
			i.mu.Lock()
			i.fds = append(i.fds, fds...)
			i.mu.Unlock()
		}
	}
	if n == 0 && err == nil {
		return 0, io.EOF
	}
	return n, err
}

func (i *Inbox) Take() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	if len(i.fds) == 0 {
		return -1
	}
	fd := i.fds[0]
	i.fds = i.fds[1:]
	return fd
}

func (i *Inbox) TakeFile(name string) *os.File {
	fd := i.Take()
	if fd < 0 {
		return nil
	}
	return os.NewFile(uintptr(fd), name)
}

func (i *Inbox) Drop() {
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, fd := range i.fds {
		unix.Close(fd)
	}
	i.fds = nil
}
