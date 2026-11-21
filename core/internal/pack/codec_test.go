package pack

import (
	"bytes"
	"io"
	"math/rand/v2"
	"testing"
	"testing/iotest"

	"github.com/ulikunitz/xz"
)

func plain(w io.Writer) (io.WriteCloser, error) {
	return xz.NewWriter(w)
}

type nop struct{ io.Writer }

func (nop) Close() error { return nil }

func code() []byte {
	r := rand.New(rand.NewPCG(1, 2))
	b := make([]byte, 3<<20+3)
	for i := range b {
		switch n := r.IntN(100); {
		case n < 6:
			b[i] = 0xe8
		case n < 9:
			b[i] = 0xe9
		case n < 30:
			b[i] = 0
		case n < 45:
			b[i] = 0xff
		default:
			b[i] = byte(r.IntN(256))
		}
	}
	copy(b[len(b)-4:], []byte{0xe8, 1, 2, 0})
	return b
}

func TestTheCallFilterComesBackExactlyWhateverTheChunks(t *testing.T) {
	src := code()
	whole := bytes.Clone(src)
	calls(whole, 0, true)
	if bytes.Equal(whole, src) {
		t.Fatal("the filter changed nothing")
	}
	r := rand.New(rand.NewPCG(3, 4))
	var enc bytes.Buffer
	f := &filter{w: nop{&enc}}
	for rest := src; len(rest) > 0; {
		n := min(1+r.IntN(9000), len(rest))
		if _, err := f.Write(rest[:n]); err != nil {
			t.Fatal(err)
		}
		rest = rest[n:]
	}
	f.Close()
	if !bytes.Equal(enc.Bytes(), whole) {
		t.Fatal("writing in pieces filters differently from the whole")
	}
	for name, r := range map[string]io.Reader{
		"whole":    bytes.NewReader(whole),
		"one byte": iotest.OneByteReader(bytes.NewReader(whole)),
		"halves":   iotest.HalfReader(bytes.NewReader(whole)),
		"with eof": iotest.DataErrReader(bytes.NewReader(whole)),
	} {
		got, err := io.ReadAll(&unfilter{r: r, buf: make([]byte, 4096)})
		if err != nil || !bytes.Equal(got, src) {
			t.Fatalf("%s: %v, equal=%v", name, err, bytes.Equal(got, src))
		}
	}
}

func TestAFailedReadIsNotTakenForTheEnd(t *testing.T) {
	src := code()[:1<<16]
	_, err := io.ReadAll(&unfilter{r: io.MultiReader(bytes.NewReader(src), iotest.ErrReader(io.ErrUnexpectedEOF)), buf: make([]byte, 4096)})
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("got %v", err)
	}
}
