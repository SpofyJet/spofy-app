package update

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

const apk = "Spofy-0.2.0-arm64.apk"

type mirror struct {
	*httptest.Server
	hits atomic.Int32
	stop chan struct{}
}

func newMirror(t *testing.T, body []byte) *mirror {
	t.Helper()
	m := &mirror{stop: make(chan struct{})}
	mux := http.NewServeMux()
	whole := func(b []byte) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", strconv.Itoa(len(b)))
			w.Write(b)
		}
	}
	streamed := func(b []byte) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Write(b[:1000])
			w.(http.Flusher).Flush()
			w.Write(b[1000:])
		}
	}
	hang := func(w http.ResponseWriter, r *http.Request) {
		w.Write(body[:len(body)/4])
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-m.stop:
		}
	}
	flipped := bytes.Clone(body)
	flipped[len(flipped)/2] ^= 1
	mux.HandleFunc("/good/", whole(body))
	mux.HandleFunc("/flipped/", whole(flipped))
	mux.HandleFunc("/longer/", streamed(append(bytes.Clone(body), make([]byte, 5000)...)))
	mux.HandleFunc("/shorter/", streamed(body[:len(body)-5000]))
	mux.HandleFunc("/announced/", whole(append(bytes.Clone(body), 0)))
	mux.HandleFunc("/cut/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Write(body[:len(body)/2])
	})
	mux.HandleFunc("/missing/", http.NotFound)
	mux.HandleFunc("/broken/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	})
	mux.HandleFunc("/hang/", hang)
	mux.HandleFunc("/slow/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		for part := range slices.Chunk(body, len(body)/10+1) {
			w.Write(part)
			w.(http.Flusher).Flush()
			time.Sleep(80 * time.Millisecond)
		}
	})
	m.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.hits.Add(1)
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(m.Close)
	t.Cleanup(func() { close(m.stop) })
	return m
}

func (m *mirror) at(route string) string {
	return m.URL + "/" + route + "/" + apk
}

func payload(t *testing.T) []byte {
	t.Helper()
	b := make([]byte, 300<<10)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func fileOf(body []byte, urls ...string) File {
	sum := sha256.Sum256(body)
	return File{Name: apk, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), URLs: urls}
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

type progressLog struct {
	done, total []int64
}

func (p *progressLog) add(done, total int64) {
	p.done = append(p.done, done)
	p.total = append(p.total, total)
}

func (p *progressLog) mustClimbTo(t *testing.T, size int64) {
	t.Helper()
	if len(p.done) == 0 || p.done[len(p.done)-1] != size {
		t.Fatalf("progress %v does not end at %d", p.done, size)
	}
	for i := range p.done {
		if p.total[i] != size || i > 0 && p.done[i] <= p.done[i-1] {
			t.Fatalf("progress step %d: %d of %d after %v", i, p.done[i], p.total[i], p.done[:i])
		}
	}
}

func TestDownloadSavesTheWholeFileForTheOwnerOnly(t *testing.T) {
	body := payload(t)
	m := newMirror(t, body)
	dir := filepath.Join(t.TempDir(), "updates")
	var p progressLog
	path, err := Download(context.Background(), m.Client(), fileOf(body, m.at("good")), dir, p.add)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, apk) {
		t.Fatalf("path %s", path)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("saved %d bytes, err = %v", len(got), err)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v, err = %v", fi.Mode(), err)
	}
	if n := names(t, dir); !slices.Equal(n, []string{apk}) {
		t.Fatalf("folder holds %v", n)
	}
	if len(p.done) < 2 {
		t.Fatalf("progress reported %d times", len(p.done))
	}
	p.mustClimbTo(t, int64(len(body)))
}

func TestDownloadKeepsNothingFromABadAnswer(t *testing.T) {
	body := payload(t)
	m := newMirror(t, body)
	for _, route := range []string{"flipped", "longer", "shorter", "announced", "cut", "missing", "broken"} {
		dir := t.TempDir()
		path, err := Download(context.Background(), m.Client(), fileOf(body, m.at(route)), dir, nil)
		if err == nil || path != "" {
			t.Fatalf("%s: path %q, err = %v", route, path, err)
		}
		if route == "longer" && !errors.Is(err, errTooLong) {
			t.Fatalf("%s: err = %v, want %v", route, err, errTooLong)
		}
		if n := names(t, dir); len(n) != 0 {
			t.Fatalf("%s: folder holds %v", route, n)
		}
	}
}

func TestDownloadReadsNothingFromAServerAnnouncingAnotherSize(t *testing.T) {
	body := payload(t)
	m := newMirror(t, body)
	var p progressLog
	_, err := Download(context.Background(), m.Client(), fileOf(body, m.at("announced")), t.TempDir(), p.add)
	if err == nil || len(p.done) != 0 {
		t.Fatalf("progress %v, err = %v", p.done, err)
	}
}

func TestAFullDiskStopsTheDownloadInsteadOfTryingEveryLink(t *testing.T) {
	full, err := os.OpenFile("/dev/full", os.O_WRONLY, 0)
	if err != nil {
		t.Skip("no /dev/full here")
	}
	defer full.Close()
	body := payload(t)
	m := newMirror(t, body)
	f := fileOf(body, m.at("good"), m.at("good"))
	want, _ := hex.DecodeString(f.SHA256)
	err = pull(context.Background(), m.Client(), f, want, full, func(int64, int64) {})
	if !errors.Is(err, syscall.ENOSPC) || m.hits.Load() != 1 {
		t.Fatalf("err = %v, mirror asked %d times", err, m.hits.Load())
	}
}

func TestDownloadTriesTheNextLinkWhenOneFails(t *testing.T) {
	body := payload(t)
	m := newMirror(t, body)
	dir := t.TempDir()
	var p progressLog
	f := fileOf(body, refused(t), m.at("broken"), m.at("cut"), m.at("flipped"), m.at("good"))
	path, err := Download(context.Background(), m.Client(), f, dir, p.add)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, body) {
		t.Fatal("saved file differs")
	}
	if n := names(t, dir); !slices.Equal(n, []string{apk}) {
		t.Fatalf("folder holds %v", n)
	}
	p.mustClimbTo(t, int64(len(body)))
}

func TestDownloadSaysWhatWentWrongWithEveryLink(t *testing.T) {
	body := payload(t)
	m := newMirror(t, body)
	_, err := Download(context.Background(), m.Client(), fileOf(body, m.at("broken"), m.at("flipped")), t.TempDir(), nil)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("http status 500")) || !bytes.Contains([]byte(err.Error()), []byte("sha256")) {
		t.Fatalf("err = %v", err)
	}
}

func TestDownloadLeavesNoFileWhenCancelledMidway(t *testing.T) {
	body := payload(t)
	m := newMirror(t, body)
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	progress := func(done, total int64) {
		if done > 0 {
			cancel()
		}
	}
	path, err := Download(ctx, m.Client(), fileOf(body, m.at("hang"), m.at("good")), dir, progress)
	if !errors.Is(err, context.Canceled) || path != "" {
		t.Fatalf("path %q, err = %v", path, err)
	}
	if n := names(t, dir); len(n) != 0 {
		t.Fatalf("folder holds %v", n)
	}
	if hits := m.hits.Load(); hits != 1 {
		t.Fatalf("mirror asked %d times after the cancel", hits)
	}
}

func TestDownloadLeavesAStalledLinkForTheNextButWaitsForASlowOne(t *testing.T) {
	defer func(d time.Duration) { stallAfter = d }(stallAfter)
	stallAfter = 300 * time.Millisecond
	body := payload(t)
	m := newMirror(t, body)
	began := time.Now()
	path, err := Download(context.Background(), m.Client(), fileOf(body, m.at("hang"), m.at("slow")), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, body) {
		t.Fatal("saved file differs")
	}
	if d := time.Since(began); d < 2*stallAfter || d > 5*time.Second {
		t.Fatalf("took %v", d)
	}
	if hits := m.hits.Load(); hits != 2 {
		t.Fatalf("mirror asked %d times", hits)
	}
	if _, err := Download(context.Background(), m.Client(), fileOf(body, m.at("hang")), t.TempDir(), nil); !errors.Is(err, errStalled) {
		t.Fatalf("err = %v, want %v", err, errStalled)
	}
}

func TestDownloadReusesAGoodFileWithoutAsking(t *testing.T) {
	body := payload(t)
	m := newMirror(t, body)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, apk), body, 0o600); err != nil {
		t.Fatal(err)
	}
	var p progressLog
	path, err := Download(context.Background(), m.Client(), fileOf(body, m.at("good")), dir, p.add)
	if err != nil || path != filepath.Join(dir, apk) {
		t.Fatalf("path %q, err = %v", path, err)
	}
	if hits := m.hits.Load(); hits != 0 {
		t.Fatalf("mirror asked %d times", hits)
	}
	p.mustClimbTo(t, int64(len(body)))
}

func TestDownloadReplacesAFileThatDoesNotMatch(t *testing.T) {
	body := payload(t)
	m := newMirror(t, body)
	dir := t.TempDir()
	for _, old := range [][]byte{make([]byte, len(body)), []byte("old")} {
		if err := os.WriteFile(filepath.Join(dir, apk), old, 0o600); err != nil {
			t.Fatal(err)
		}
		path, err := Download(context.Background(), m.Client(), fileOf(body, m.at("good")), dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(path); !bytes.Equal(got, body) {
			t.Fatal("the old file was kept")
		}
	}
}

func TestDownloadRefusesAFileEntryThatIsNotSafe(t *testing.T) {
	body := payload(t)
	m := newMirror(t, body)
	dir := filepath.Join(t.TempDir(), "updates")
	for _, edit := range []func(*File){
		func(f *File) { f.Name = "../" + apk },
		func(f *File) { f.Name = "sub/" + apk },
		func(f *File) { f.Name = `sub\` + apk },
		func(f *File) { f.Name = ".." },
		func(f *File) { f.Name = "." },
		func(f *File) { f.Name = "" },
		func(f *File) { f.Name = "C:" + apk },
		func(f *File) { f.Name = "/etc/" + apk },
		func(f *File) { f.URLs = []string{"http" + f.URLs[0][len("https"):]} },
		func(f *File) { f.SHA256 = "" },
	} {
		f := fileOf(body, m.at("good"))
		edit(&f)
		if _, err := Download(context.Background(), m.Client(), f, dir, nil); !errors.Is(err, ErrManifest) {
			t.Fatalf("%+v: err = %v, want %v", f, err, ErrManifest)
		}
	}
	if hits := m.hits.Load(); hits != 0 {
		t.Fatalf("mirror asked %d times", hits)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("the folder was made: %v", err)
	}
}

func TestDownloadClearsWhatAKilledDownloadLeftBehind(t *testing.T) {
	body := payload(t)
	m := newMirror(t, body)
	dir := t.TempDir()
	for name, b := range map[string][]byte{".update-123456": body[:1000], "notes.txt": []byte("keep")} {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Download(context.Background(), m.Client(), fileOf(body, m.at("good")), dir, nil); err != nil {
		t.Fatal(err)
	}
	if n := names(t, dir); !slices.Equal(n, []string{apk, "notes.txt"}) {
		t.Fatalf("folder holds %v", n)
	}
}

func TestDownloadsIntoOneFolderAtOnceEndWithOneGoodFile(t *testing.T) {
	body := payload(t)
	m := newMirror(t, body)
	dir := t.TempDir()
	f := fileOf(body, m.at("good"))
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Go(func() {
			_, errs[i] = Download(context.Background(), m.Client(), f, dir, nil)
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
	}
	if n := names(t, dir); !slices.Equal(n, []string{apk}) {
		t.Fatalf("folder holds %v", n)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, apk)); !bytes.Equal(got, body) {
		t.Fatal("saved file differs")
	}
	if hits := m.hits.Load(); hits != 1 {
		t.Fatalf("mirror asked %d times", hits)
	}
}
