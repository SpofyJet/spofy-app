package pack

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	magic       = "SPOFYPK1"
	versionSize = 32
	trailerSize = 3*8 + versionSize + sha256.Size + len(magic)
	align       = 8
)

var ErrDamaged = errors.New("pack: the installer is damaged or incomplete")

type Trailer struct {
	Offset   int64
	Size     int64
	Unpacked int64
	Version  string
	Sum      [sha256.Size]byte
}

func (t Trailer) encode() []byte {
	b := make([]byte, 0, trailerSize)
	b = binary.LittleEndian.AppendUint64(b, uint64(t.Offset))
	b = binary.LittleEndian.AppendUint64(b, uint64(t.Size))
	b = binary.LittleEndian.AppendUint64(b, uint64(t.Unpacked))
	var v [versionSize]byte
	copy(v[:], t.Version)
	b = append(b, v[:]...)
	b = append(b, t.Sum[:]...)
	return append(b, magic...)
}

func Write(out io.Writer, stub io.Reader, dir, version string, compress Compressor) (Trailer, error) {
	t := Trailer{Version: version}
	if len(version) == 0 || len(version) > versionSize || strings.ContainsRune(version, 0) {
		return t, fmt.Errorf("pack: the version %q does not fit", version)
	}
	names, err := files(dir)
	if err != nil {
		return t, err
	}
	var payload bytes.Buffer
	z, err := compress(&payload)
	if err != nil {
		return t, err
	}
	f := &filter{w: z}
	tw := tar.NewWriter(f)
	for _, name := range names {
		n, err := put(tw, dir, name)
		if err != nil {
			z.Close()
			return t, err
		}
		t.Unpacked += n
	}
	if err := tw.Close(); err != nil {
		z.Close()
		return t, err
	}
	if err := f.Close(); err != nil {
		return t, err
	}
	head, err := io.Copy(out, stub)
	if err != nil {
		return t, err
	}
	t.Size = int64(payload.Len())
	pad := (align - (head+t.Size+int64(trailerSize))%align) % align
	t.Offset = head + pad
	t.Sum = sha256.Sum256(payload.Bytes())
	for _, b := range [][]byte{make([]byte, pad), payload.Bytes(), t.encode()} {
		if _, err := out.Write(b); err != nil {
			return t, err
		}
	}
	return t, nil
}

func files(dir string) ([]string, error) {
	var names []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir():
			return nil
		case !d.Type().IsRegular():
			return fmt.Errorf("pack: %s is not a regular file", path)
		}
		rel, err := filepath.Rel(dir, path)
		names = append(names, filepath.ToSlash(rel))
		return err
	})
	if err == nil && len(names) == 0 {
		err = errors.New("pack: nothing to pack in " + dir)
	}
	sort.Strings(names)
	return names, err
}

func put(tw *tar.Writer, dir, name string) (int64, error) {
	f, err := os.Open(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Size: info.Size(), Mode: 0o644, Format: tar.FormatPAX}); err != nil {
		return 0, err
	}
	return io.Copy(tw, f)
}

type Archive struct {
	Trailer
	r io.ReaderAt
}

func Open(r io.ReaderAt, size int64) (*Archive, error) {
	end := unsigned(r, size)
	var tail [trailerSize + align - 1]byte
	from := max(end-int64(len(tail)), 0)
	n, err := r.ReadAt(tail[:end-from], from)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	b := bytes.TrimRight(tail[:n], "\x00")
	if len(b) < trailerSize || n-len(b) >= align || !bytes.HasSuffix(b, []byte(magic)) {
		return nil, ErrDamaged
	}
	start := from + int64(len(b)-trailerSize)
	b = b[len(b)-trailerSize:]
	a := &Archive{r: r}
	a.Offset = int64(binary.LittleEndian.Uint64(b))
	a.Size = int64(binary.LittleEndian.Uint64(b[8:]))
	a.Unpacked = int64(binary.LittleEndian.Uint64(b[16:]))
	a.Version = string(bytes.TrimRight(b[24:24+versionSize], "\x00"))
	copy(a.Sum[:], b[24+versionSize:])
	if a.Offset <= 0 || a.Size <= 0 || a.Unpacked <= 0 || a.Offset > start || a.Size != start-a.Offset || a.Version == "" {
		return nil, ErrDamaged
	}
	return a, nil
}

func unsigned(r io.ReaderAt, size int64) int64 {
	f, err := pe.NewFile(io.NewSectionReader(r, 0, size))
	if err != nil {
		return size
	}
	var dirs []pe.DataDirectory
	switch h := f.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		dirs = h.DataDirectory[:min(h.NumberOfRvaAndSizes, uint32(len(h.DataDirectory)))]
	case *pe.OptionalHeader32:
		dirs = h.DataDirectory[:min(h.NumberOfRvaAndSizes, uint32(len(h.DataDirectory)))]
	}
	if len(dirs) <= pe.IMAGE_DIRECTORY_ENTRY_SECURITY {
		return size
	}
	if d := dirs[pe.IMAGE_DIRECTORY_ENTRY_SECURITY]; d.VirtualAddress != 0 && int64(d.VirtualAddress)+int64(d.Size) == size {
		return int64(d.VirtualAddress)
	}
	return size
}

func (a *Archive) Verify() error {
	sum := sha256.New()
	if _, err := io.Copy(sum, io.NewSectionReader(a.r, a.Offset, a.Size)); err != nil {
		return err
	}
	if !bytes.Equal(sum.Sum(nil), a.Sum[:]) {
		return ErrDamaged
	}
	return nil
}

func (a *Archive) Extract(into string, progress func(done, total int64) error) error {
	z, err := decompress(io.NewSectionReader(a.r, a.Offset, a.Size))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDamaged, err)
	}
	tr := tar.NewReader(z)
	var done int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: %v", ErrDamaged, err)
		}
		name := filepath.FromSlash(h.Name)
		if h.Typeflag != tar.TypeReg || !filepath.IsLocal(name) || h.Size < 0 || done+h.Size > a.Unpacked {
			return fmt.Errorf("%w: entry %q", ErrDamaged, h.Name)
		}
		n, err := write(filepath.Join(into, name), tr, h.Size, func(n int64) error {
			if progress == nil {
				return nil
			}
			return progress(done+n, a.Unpacked)
		})
		done += n
		if err != nil {
			return err
		}
	}
	if done != a.Unpacked {
		return fmt.Errorf("%w: %d of %d bytes", ErrDamaged, done, a.Unpacked)
	}
	return nil
}

func write(path string, r io.Reader, size int64, progress func(int64) error) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return 0, err
	}
	n, err := io.CopyBuffer(&reporter{w: f, report: progress}, io.LimitReader(r, size), make([]byte, 1<<20))
	if err == nil && n != size {
		err = fmt.Errorf("%w: %s is short", ErrDamaged, path)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return n, err
}

type reporter struct {
	w      io.Writer
	n      int64
	report func(int64) error
}

func (r *reporter) Write(p []byte) (int, error) {
	n, err := r.w.Write(p)
	r.n += int64(n)
	if err == nil {
		err = r.report(r.n)
	}
	return n, err
}
