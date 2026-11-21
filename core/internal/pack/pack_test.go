package pack

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func bundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"spofy.exe":                       strings.Repeat("window ", 5000),
		"spofy-core.exe":                  strings.Repeat("core ", 20000),
		"data/flutter_assets/fonts/a.ttf": "font",
		"data/app.so":                     "",
	} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func packed(t *testing.T, stub []byte) ([]byte, Trailer) {
	t.Helper()
	var out bytes.Buffer
	tr, err := Write(&out, bytes.NewReader(stub), bundle(t), "1.2.3.4", plain)
	if err != nil {
		t.Fatal(err)
	}
	return out.Bytes(), tr
}

func TestWhatIsPackedComesBackWhole(t *testing.T) {
	stub := []byte("MZ not a real program")
	b, tr := packed(t, stub)
	if pad := tr.Offset - int64(len(stub)); !bytes.HasPrefix(b, stub) || pad < 0 || pad >= align || tr.Offset+tr.Size+int64(trailerSize) != int64(len(b)) || len(b)%align != 0 {
		t.Fatalf("layout: offset %d, size %d", tr.Offset, len(b))
	}
	a, err := Open(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if a.Trailer != tr || a.Version != "1.2.3.4" {
		t.Fatalf("trailer %+v, want %+v", a.Trailer, tr)
	}
	if err := a.Verify(); err != nil {
		t.Fatal(err)
	}
	into := t.TempDir()
	var last, total int64
	if err := a.Extract(into, func(done, all int64) error { last, total = done, all; return nil }); err != nil {
		t.Fatal(err)
	}
	if last != total || total != a.Unpacked {
		t.Fatalf("progress ended at %d of %d, unpacked %d", last, total, a.Unpacked)
	}
	src := bundle(t)
	filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		want, _ := os.ReadFile(path)
		got, err := os.ReadFile(filepath.Join(into, rel))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s: %v", rel, err)
		}
		return nil
	})
	if err := a.Extract(into, nil); err == nil {
		t.Fatal("unpacking over files that are there must fail, not mix two versions")
	}
	stop := errors.New("stop")
	calls := 0
	if err := a.Extract(t.TempDir(), func(int64, int64) error { calls++; return stop }); !errors.Is(err, stop) || calls != 1 {
		t.Fatalf("a refusal of progress must stop the unpacking at once: %v after %d calls", err, calls)
	}
}

func TestADamagedInstallerIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	b, tr := packed(t, []byte("stub"))
	for name, broken := range map[string][]byte{
		"cut in the middle":    b[:len(b)/2],
		"cut at the end":       b[:len(b)-1],
		"nothing appended":     []byte("stub"),
		"flipped payload byte": flip(b, int(tr.Offset)+int(tr.Size)/2),
		"flipped trailer byte": flip(b, len(b)-trailerSize+3),
	} {
		a, err := Open(bytes.NewReader(broken), int64(len(broken)))
		if err == nil {
			err = a.Verify()
		}
		if !errors.Is(err, ErrDamaged) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestASignatureAfterThePayloadIsSkipped(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	stub, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(bytes.NewReader(stub), int64(len(stub))); !errors.Is(err, ErrDamaged) {
		t.Fatalf("a program without a payload: %v", err)
	}
	b, _ := packed(t, stub)
	if !strings.HasPrefix(string(stub), "MZ") {
		t.Skip("this system builds no Windows programs, the signature is checked on Windows")
	}
	signed := sign(t, b)
	a, err := Open(bytes.NewReader(signed), int64(len(signed)))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Verify(); err != nil {
		t.Fatal(err)
	}
	tampered := append(bytes.Clone(signed), 0)
	if _, err := Open(bytes.NewReader(tampered), int64(len(tampered))); !errors.Is(err, ErrDamaged) {
		t.Fatalf("bytes after the signature: %v", err)
	}
}

func TestThePackIsTheSameEveryTime(t *testing.T) {
	dir := bundle(t)
	var one, two bytes.Buffer
	if _, err := Write(&one, strings.NewReader("stub"), dir, "1", plain); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(&two, strings.NewReader("stub"), dir, "1", plain); err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(one.Bytes()) != sha256.Sum256(two.Bytes()) {
		t.Fatal("two packs of the same folder differ")
	}
}

func sign(t *testing.T, b []byte) []byte {
	t.Helper()
	at := int(binary.LittleEndian.Uint32(b[0x3c:])) + 4 + 20
	if binary.LittleEndian.Uint16(b[at:]) != 0x20b {
		t.Fatal("the stub is not a 64-bit program")
	}
	certificate := append(binary.LittleEndian.AppendUint32(nil, 24), 0, 2, 2, 0)
	certificate = append(certificate, bytes.Repeat([]byte{0x30}, 16)...)
	signed := append(bytes.Clone(b), certificate...)
	binary.LittleEndian.PutUint32(signed[at+144:], uint32(len(b)))
	binary.LittleEndian.PutUint32(signed[at+148:], uint32(len(certificate)))
	return signed
}

func flip(b []byte, at int) []byte {
	c := bytes.Clone(b)
	c[at] ^= 0xff
	return c
}
