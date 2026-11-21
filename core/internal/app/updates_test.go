package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SpofyJet/spofy-app/core/internal/app/apptest"
)

func TestAnUpdateIsOfferedDownloadedAndChecked(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "updates")
	var rel *apptest.Releases
	b := newBench(t, func(c *Config) {
		rel = apptest.NewReleases(t, nil)
		c.Build = 1
		c.Engine = "xray-test"
		c.Updates = Updates{Sources: []string{apptest.ReleasesURL}, Keys: []ed25519.PublicKey{rel.Public()}, Platform: apptest.Platform, Dir: dir, Every: -1}
	})
	b.panel.Route(apptest.ReleasesHost, rel)
	priv := rel.Key
	if s := b.app.Snapshot(); s.Update != nil || s.Build != 1 || s.Engine != "xray-test" {
		t.Fatalf("before: %+v %d %q", s.Update, s.Build, s.Engine)
	}
	apk := bytes.Repeat([]byte("spofy-apk "), 40000)
	rel.Publish(2, 100, apk, apk)
	b.seen()
	if err := b.app.CheckUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	same(t, b.seen(), []string{EvUpdate})
	u := b.app.Snapshot().Update
	if u == nil || u.Version != "0.2.0" || u.Build != 2 || u.Size != int64(len(apk)) || u.Notes != "Что нового" || u.File != "" || u.Required {
		t.Fatalf("offer %+v", u)
	}
	if err := b.app.FetchUpdate(); err != nil {
		t.Fatal(err)
	}
	seen := b.seen()
	if len(seen) < 2 || len(seen) > 102 {
		t.Fatalf("%d progress events", len(seen))
	}
	u = b.app.Snapshot().Update
	got, err := os.ReadFile(u.File)
	if err != nil || !bytes.Equal(got, apk) || u.Done != u.Size || filepath.Dir(u.File) != dir {
		t.Fatalf("file %q done %d of %d: %v", u.File, u.Done, u.Size, err)
	}
	var ran []string
	b.app.cfg.Install = func(path string) error { ran = append(ran, path); return nil }
	if err := b.app.InstallUpdate(); err != nil || len(ran) != 1 || ran[0] != u.File {
		t.Fatalf("install: %v %v", err, ran)
	}
	os.WriteFile(u.File, append(bytes.Clone(apk), '!'), 0o600)
	if err := b.app.InstallUpdate(); code(err) != CodeUpdateFile || len(ran) != 1 {
		t.Fatalf("a file changed after the check was run: %v %v", err, ran)
	}
	if u := b.app.Snapshot().Update; u.File != "" || u.Done != 0 {
		t.Fatalf("after the changed file: %+v", u)
	}
	if err := b.app.FetchUpdate(); err != nil {
		t.Fatal(err)
	}
	broken := bytes.Clone(apk)
	broken[100] ^= 1
	rel.Publish(3, 100, apk, broken)
	if err := b.app.CheckUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(u.File); !os.IsNotExist(err) {
		t.Fatal("the file of the older offer was kept")
	}
	if err := b.app.FetchUpdate(); code(err) != CodeUpdateFile {
		t.Fatalf("a file that differs from the manifest: %v", err)
	}
	if u := b.app.Snapshot().Update; u.Build != 3 || u.File != "" {
		t.Fatalf("after a broken download: %+v", u)
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Fatalf("left in the folder: %v", left)
	}
	_, stranger, _ := ed25519.GenerateKey(rand.Reader)
	rel.Key = stranger
	rel.Publish(4, 100, apk, apk)
	if err := b.app.CheckUpdate(context.Background()); code(err) != CodeUpdateBroken {
		t.Fatalf("a manifest signed by another key: %v", err)
	}
	rel.Key = priv
	rel.Publish(1, 100, apk, apk)
	if err := b.app.CheckUpdate(context.Background()); code(err) != CodeUpdateNone || b.app.Snapshot().Update != nil {
		t.Fatalf("the same build: %v %+v", err, b.app.Snapshot().Update)
	}
	if !strings.Contains(fmt.Sprint(b.app.Logs()), "update check failed: update_broken") {
		t.Fatal("the broken signature is not in the log book")
	}
}

func TestUpdatesAreOffWithoutKeysOrBuild(t *testing.T) {
	b := newBench(t, nil)
	if err := b.app.CheckUpdate(context.Background()); code(err) != CodeUpdateOff {
		t.Fatalf("%v", err)
	}
	if err := b.app.FetchUpdate(); code(err) != CodeUpdateNone {
		t.Fatalf("%v", err)
	}
	if err := b.app.InstallUpdate(); code(err) != CodeUpdateOff {
		t.Fatalf("%v", err)
	}
}
