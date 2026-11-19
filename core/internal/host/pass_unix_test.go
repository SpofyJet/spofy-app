//go:build unix

package host

import (
	"bufio"
	"net"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func number(t *testing.T, f *os.File) int {
	t.Helper()
	raw, err := f.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	n := -1
	raw.Control(func(fd uintptr) { n = int(fd) })
	return n
}

func same(t *testing.T, a, b int) bool {
	t.Helper()
	var x, y unix.Stat_t
	if unix.Fstat(a, &x) != nil || unix.Fstat(b, &y) != nil {
		t.Fatal("fstat")
	}
	return x.Dev == y.Dev && x.Ino == y.Ino
}

func TestDescriptorsTravelWithTheirLines(t *testing.T) {
	near, far, err := Pair()
	if err != nil {
		t.Fatal(err)
	}
	defer near.Close()
	conn, err := net.FileConn(far)
	far.Close()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	in := NewInbox(conn.(*net.UnixConn))
	first, second := pipe(t), pipe(t)
	if _, err := near.Write([]byte("{\"n\":1}\n")); err != nil {
		t.Fatal(err)
	}
	for _, f := range []*os.File{first, second} {
		if err := Send(near, []byte("{\"n\":2}\n"), f); err != nil {
			t.Fatal(err)
		}
	}
	sc := bufio.NewScanner(in)
	for n, want := range []*os.File{nil, first, second} {
		if !sc.Scan() {
			t.Fatalf("line %d: %v", n+1, sc.Err())
		}
		if want == nil {
			continue
		}
		got := in.TakeFile("got")
		if got == nil {
			t.Fatalf("line %d came without its descriptor", n+1)
		}
		flags, _ := unix.FcntlInt(uintptr(number(t, got)), unix.F_GETFD, 0)
		if !same(t, number(t, got), number(t, want)) || flags&unix.FD_CLOEXEC == 0 {
			t.Fatalf("line %d: another file or no close-on-exec (flags %d)", n+1, flags)
		}
		got.Close()
	}
	if in.Take() >= 0 {
		t.Fatal("a descriptor nobody sent")
	}
	if err := Send(near, []byte("{\"n\":4}\n"), first); err != nil {
		t.Fatal(err)
	}
	sc.Scan()
	in.Drop()
	if in.Take() >= 0 {
		t.Fatal("a dropped descriptor is still there")
	}
	near.Close()
	if sc.Scan() {
		t.Fatal("a line after the end")
	}
}

func TestSendKeepsTheFileNonBlocking(t *testing.T) {
	near, far, err := Pair()
	if err != nil {
		t.Fatal(err)
	}
	defer near.Close()
	defer far.Close()
	var fds [2]int
	if err := unix.Pipe2(fds[:], unix.O_NONBLOCK|unix.O_CLOEXEC); err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fds[1])
	file := os.NewFile(uintptr(fds[0]), "tun")
	defer file.Close()
	if err := Send(near, []byte("x\n"), file); err != nil {
		t.Fatal(err)
	}
	if flags, _ := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFL, 0); flags&unix.O_NONBLOCK == 0 {
		t.Fatal("sending made the file blocking")
	}
}
