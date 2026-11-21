package pack

import (
	"bufio"
	"errors"
	"io"

	"github.com/ulikunitz/xz"
)

type Compressor func(io.Writer) (io.WriteCloser, error)

func calls(b []byte, at uint32, encode bool) int {
	i := 0
	for i+5 <= len(b) {
		if b[i] != 0xe8 && b[i] != 0xe9 {
			i++
			continue
		}
		if b[i+4] == 0 || b[i+4] == 0xff {
			v := uint32(b[i+1]) | uint32(b[i+2])<<8 | uint32(b[i+3])<<16 | uint32(b[i+4]&1)<<24
			if encode {
				v += at + uint32(i) + 5
			} else {
				v -= at + uint32(i) + 5
			}
			b[i+1], b[i+2], b[i+3], b[i+4] = byte(v), byte(v>>8), byte(v>>16), -byte(v>>24&1)
		}
		i += 5
	}
	return i
}

type filter struct {
	w   io.WriteCloser
	buf []byte
	at  uint32
}

func (f *filter) Write(p []byte) (int, error) {
	f.buf = append(f.buf, p...)
	i := calls(f.buf, f.at, true)
	if _, err := f.w.Write(f.buf[:i]); err != nil {
		return 0, err
	}
	f.at += uint32(i)
	f.buf = append(f.buf[:0], f.buf[i:]...)
	return len(p), nil
}

func (f *filter) Close() error {
	_, err := f.w.Write(f.buf)
	return errors.Join(err, f.w.Close())
}

type unfilter struct {
	r         io.Reader
	buf       []byte
	out, pend []byte
	at        uint32
	err       error
}

func (u *unfilter) Read(p []byte) (int, error) {
	for len(u.out) == 0 {
		if u.err != nil {
			if len(u.pend) == 0 {
				return 0, u.err
			}
			u.out, u.pend = u.pend, nil
			break
		}
		k := copy(u.buf, u.pend)
		n, err := u.r.Read(u.buf[k:])
		u.err = err
		k += n
		i := calls(u.buf[:k], u.at, false)
		u.at += uint32(i)
		u.out, u.pend = u.buf[:i], u.buf[i:k]
	}
	n := copy(p, u.out)
	u.out = u.out[n:]
	return n, nil
}

func decompress(r io.Reader) (io.Reader, error) {
	z, err := xz.NewReader(bufio.NewReaderSize(r, 1<<20))
	if err != nil {
		return nil, err
	}
	return &unfilter{r: z, buf: make([]byte, 1<<20)}, nil
}
