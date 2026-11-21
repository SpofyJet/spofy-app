package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const tempPrefix = ".update-"

var stallAfter = 30 * time.Second

var (
	errStalled = errors.New("no data for too long")
	errTooLong = errors.New("server sends more than the manifest says")
)

var busy = make(chan struct{}, 1)

func Download(ctx context.Context, c *http.Client, f File, dir string, progress func(done, total int64)) (string, error) {
	if err := f.valid(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrManifest, err)
	}
	select {
	case busy <- struct{}{}:
		defer func() { <-busy }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if progress == nil {
		progress = func(int64, int64) {}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	sweep(dir)
	want, _ := hex.DecodeString(f.SHA256)
	path := filepath.Join(dir, f.Name)
	if same(path, f.Size, want) {
		progress(f.Size, f.Size)
		return path, nil
	}
	tmp, err := os.CreateTemp(dir, tempPrefix+"*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err := pull(ctx, c, f, want, tmp, progress); err != nil {
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	return path, nil
}

func pull(ctx context.Context, c *http.Client, f File, want []byte, tmp *os.File, progress func(done, total int64)) error {
	m := &meter{total: f.Size, report: progress}
	var errs tried
	for _, u := range f.URLs {
		err := fill(ctx, c, u, want, &sink{file: tmp, hash: sha256.New(), size: f.Size, meter: m})
		var local *writeError
		switch {
		case err == nil:
			return nil
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.As(err, &local):
			return local.err
		}
		errs = append(errs, fmt.Errorf("%s: %w", u, err))
	}
	return fmt.Errorf("update: download failed: %w", errs)
}

func fill(ctx context.Context, c *http.Client, u string, want []byte, s *sink) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	s.watch = time.AfterFunc(stallAfter, func() { cancel(errStalled) })
	defer s.watch.Stop()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return bare(err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("http status %d", resp.StatusCode)
	case resp.ContentLength >= 0 && resp.ContentLength != s.size:
		return fmt.Errorf("server announces %d bytes, want %d", resp.ContentLength, s.size)
	}
	if _, err := io.Copy(s, resp.Body); err != nil {
		return err
	}
	switch {
	case s.done != s.size:
		return fmt.Errorf("server sent %d bytes, want %d", s.done, s.size)
	case !bytes.Equal(s.hash.Sum(nil), want):
		return errors.New("sha256 does not match")
	}
	return nil
}

func same(path string, size int64, want []byte) bool {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() != size {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return bytes.Equal(h.Sum(nil), want)
}

func sweep(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), tempPrefix) && e.Type().IsRegular() {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

type meter struct {
	shown, total int64
	report       func(done, total int64)
}

func (m *meter) at(done int64) {
	if done > m.shown {
		m.shown = done
		m.report(done, m.total)
	}
}

type sink struct {
	file  *os.File
	hash  hash.Hash
	size  int64
	done  int64
	meter *meter
	watch *time.Timer
}

func (s *sink) Write(b []byte) (int, error) {
	if s.done+int64(len(b)) > s.size {
		return 0, errTooLong
	}
	if _, err := s.file.WriteAt(b, s.done); err != nil {
		return 0, &writeError{err}
	}
	s.hash.Write(b)
	s.done += int64(len(b))
	s.watch.Reset(stallAfter)
	s.meter.at(s.done)
	return len(b), nil
}

type writeError struct {
	err error
}

func (e *writeError) Error() string {
	return e.err.Error()
}

func (e *writeError) Unwrap() error {
	return e.err
}
