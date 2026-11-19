package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/sub"
)

const (
	secretURL = "https://sub.example.test/TOKEN-0123456789abcdef"
	secretKey = "00000000-0000-4000-8000-000000000042"
)

func key(b byte) []byte {
	return bytes.Repeat([]byte{b}, 32)
}

func fill(st *State) {
	st.URL = secretURL
	st.Profile = &sub.Profile{
		Title:       "Spofy",
		UpdateEvery: time.Hour,
		ETag:        `W/"1"`,
		Servers:     []sub.Server{{ID: "de", Name: "DE", Config: json.RawMessage(`{"outbounds":[{"settings":{"id":"` + secretKey + `"}}]}`)}},
	}
	st.FetchedAt = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	st.Server = "de"
	st.Settings = map[string]json.RawMessage{"killSwitch": json.RawMessage(`true`)}
	st.Delays = map[string]Delay{"de": {Ms: 120, At: st.FetchedAt}}
}

func saved(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir, key(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(fill); err != nil {
		t.Fatal(err)
	}
	return dir
}

func read(t *testing.T, dir, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func exists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, key(1))
	if err != nil {
		t.Fatal(err)
	}
	if st := s.Get(); st.Version != Version || st.URL != "" || st.Profile != nil {
		t.Fatalf("fresh state = %+v", st)
	}
	if exists(dir, fileMain) {
		t.Fatal("an untouched store wrote a file")
	}
	if err := s.Update(fill); err != nil {
		t.Fatal(err)
	}
	again, err := Open(dir, key(1))
	if err != nil {
		t.Fatal(err)
	}
	st := again.Get()
	if st.URL != secretURL || st.Server != "de" || st.Profile == nil || st.Profile.Servers[0].ID != "de" || st.Profile.UpdateEvery != time.Hour {
		t.Fatalf("state = %+v", st)
	}
	if st.Delays["de"].Ms != 120 || !st.FetchedAt.Equal(time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)) || string(st.Settings["killSwitch"]) != "true" {
		t.Fatalf("state = %+v", st)
	}
	if again.Recovered() {
		t.Fatal("a clean load reported recovery")
	}
}

func TestNothingReadableOnDisk(t *testing.T) {
	dir := saved(t)
	s, _ := Open(dir, key(1))
	s.Update(func(st *State) { st.Server = "" })
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("files = %v, err %v", entries, err)
	}
	for _, e := range entries {
		data := read(t, dir, e.Name())
		for _, secret := range []string{secretURL, "TOKEN", secretKey, "outbounds", "killSwitch"} {
			if bytes.Contains(data, []byte(secret)) {
				t.Fatalf("%s contains %q in the clear", e.Name(), secret)
			}
		}
		info, _ := e.Info()
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %v", e.Name(), info.Mode().Perm())
		}
	}
	if info, _ := os.Stat(dir); runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v", info.Mode().Perm())
	}
}

func TestWrongKeyChangesNothing(t *testing.T) {
	dir := saved(t)
	before := read(t, dir, fileMain)
	if _, err := Open(dir, key(2)); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("err = %v", err)
	}
	if !bytes.Equal(before, read(t, dir, fileMain)) {
		t.Fatal("a failed open changed the file")
	}
	if _, err := Open(dir, []byte("short")); !errors.Is(err, ErrKey) {
		t.Fatalf("short key: err = %v", err)
	}
	if s, err := Open(dir, key(1)); err != nil || s.Get().URL != secretURL {
		t.Fatalf("the right key no longer works: %v", err)
	}
}

func TestDamagedFileFallsBackToPreviousCopy(t *testing.T) {
	for name, damage := range map[string]func([]byte) []byte{
		"flipped byte": func(b []byte) []byte { b[len(b)/2] ^= 1; return b },
		"cut short":    func(b []byte) []byte { return b[:len(b)/3] },
		"empty":        func([]byte) []byte { return nil },
		"wrong header": func(b []byte) []byte { copy(b, "XXXX"); return b },
	} {
		dir := saved(t)
		s, _ := Open(dir, key(1))
		if err := s.Update(func(st *State) { st.Server = "nl" }); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, fileMain), damage(read(t, dir, fileMain)), 0o600)
		got, err := Open(dir, key(1))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if st := got.Get(); st.Server != "de" || st.URL != secretURL || !got.Recovered() {
			t.Fatalf("%s: server %q, recovered %v", name, st.Server, got.Recovered())
		}
		good := read(t, dir, fileOld)
		if err := got.Update(func(st *State) { st.Server = "us" }); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(good, read(t, dir, fileOld)) {
			t.Fatalf("%s: the last good copy was replaced by the damaged file", name)
		}
		if after, err := Open(dir, key(1)); err != nil || after.Get().Server != "us" || after.Recovered() {
			t.Fatalf("%s: after the next save: %v", name, err)
		}
	}
}

func TestInterruptedSave(t *testing.T) {
	dir := saved(t)
	s, _ := Open(dir, key(1))
	s.Update(func(st *State) { st.Server = "nl" })
	os.Rename(filepath.Join(dir, fileMain), filepath.Join(dir, fileFresh))
	got, err := Open(dir, key(1))
	if err != nil || got.Get().Server != "nl" {
		t.Fatalf("save interrupted between renames: %v", err)
	}
	if !exists(dir, fileMain) || exists(dir, fileFresh) {
		t.Fatal("the interrupted save was not finished")
	}
	os.WriteFile(filepath.Join(dir, fileFresh), read(t, dir, fileMain)[:40], 0o600)
	got, err = Open(dir, key(1))
	if err != nil || got.Get().Server != "nl" || got.Recovered() {
		t.Fatalf("save interrupted while writing: %v", err)
	}
	if exists(dir, fileFresh) {
		t.Fatal("a half-written file was left behind")
	}
}

func TestUnreadableDataIsKeptUntilReset(t *testing.T) {
	dir := saved(t)
	s, _ := Open(dir, key(1))
	s.Update(func(st *State) { st.Server = "nl" })
	main, old := read(t, dir, fileMain), read(t, dir, fileOld)
	if _, err := Open(dir, key(9)); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("err = %v", err)
	}
	if !bytes.Equal(main, read(t, dir, fileMain)) || !bytes.Equal(old, read(t, dir, fileOld)) {
		t.Fatal("unreadable files were touched")
	}
	if err := Reset(dir); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(main, read(t, dir, fileMain+lostMark)) || !bytes.Equal(old, read(t, dir, fileOld+lostMark)) {
		t.Fatal("reset did not keep the old files")
	}
	fresh, err := Open(dir, key(9))
	if err != nil || fresh.Get().URL != "" {
		t.Fatalf("open after reset: %v", err)
	}
	if err := Reset(t.TempDir()); err != nil {
		t.Fatalf("reset of an empty folder: %v", err)
	}
}

func TestNewerVersionIsNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir, key(1))
	s.raw, _ = json.Marshal(map[string]any{"version": Version + 1, "future": true})
	if err := s.save(s.raw); err != nil {
		t.Fatal(err)
	}
	before := read(t, dir, fileMain)
	if _, err := Open(dir, key(1)); !errors.Is(err, ErrNewer) {
		t.Fatalf("err = %v", err)
	}
	if !bytes.Equal(before, read(t, dir, fileMain)) {
		t.Fatal("newer data was changed")
	}
}

func TestUnchangedStateIsNotRewritten(t *testing.T) {
	dir := saved(t)
	s, _ := Open(dir, key(1))
	before := read(t, dir, fileMain)
	if err := s.Update(func(st *State) { st.Server = "de" }); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(fill); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, read(t, dir, fileMain)) {
		t.Fatal("an update without changes rewrote the file")
	}
}

func TestGetReturnsACopy(t *testing.T) {
	s, _ := Open(saved(t), key(1))
	st := s.Get()
	st.Profile.Servers[0].ID = "changed"
	st.Delays["de"] = Delay{Ms: 1}
	st.Settings["killSwitch"] = json.RawMessage(`false`)
	if got := s.Get(); got.Profile.Servers[0].ID != "de" || got.Delays["de"].Ms != 120 || string(got.Settings["killSwitch"]) != "true" {
		t.Fatalf("stored state was changed through a copy: %+v", got)
	}
}

func TestConcurrentUpdates(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir, key(1))
	var wg sync.WaitGroup
	for n := 0; n < 24; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.Update(func(st *State) {
				count, _ := strconv.Atoi(string(st.Settings["count"]))
				if st.Settings == nil {
					st.Settings = map[string]json.RawMessage{}
				}
				st.Settings["count"] = json.RawMessage(strconv.Itoa(count + 1))
			})
			if err != nil {
				t.Error(err)
			}
			s.Get()
		}()
	}
	wg.Wait()
	again, err := Open(dir, key(1))
	if err != nil || string(again.Get().Settings["count"]) != "24" {
		t.Fatalf("count = %s, err %v", again.Get().Settings["count"], err)
	}
}
