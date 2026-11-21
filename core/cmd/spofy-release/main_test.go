package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/SpofyJet/spofy-app/core/internal/update"
)

func do(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := run(args, &out); err != nil {
		t.Fatalf("%s: %v", args[0], err)
	}
	return out.String()
}

func write(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func random(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAReleaseMadeByTheToolIsOfferedAndDownloadedByTheApp(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "release.key")
	printed := strings.TrimSpace(do(t, "keygen", "-out", key))
	pub, err := base64.StdEncoding.DecodeString(printed)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		t.Fatalf("keygen printed %q", printed)
	}
	apk, exe := random(t, 200<<10), random(t, 300<<10)
	write(t, filepath.Join(dir, "out", "Spofy-0.2.0-arm64.apk"), apk)
	write(t, filepath.Join(dir, "out", "Spofy-0.2.0-setup.exe"), exe)
	write(t, filepath.Join(dir, "notes.txt"), []byte("Подключение быстрее.\nМеньше расход батареи.\n"))
	ts := httptest.NewTLSServer(http.FileServer(http.Dir(dir)))
	defer ts.Close()
	do(t, "manifest", "-version", "0.2.0", "-build", "71", "-notes-file", filepath.Join(dir, "notes.txt"),
		"-min", "60", "-rollout", "100", "-url-base", ts.URL+"/out/", "-url-base", ts.URL+"/out",
		"-file", "android-arm64-v8a="+filepath.Join(dir, "out", "Spofy-0.2.0-arm64.apk"),
		"-file", "windows-x64="+filepath.Join(dir, "out", "Spofy-0.2.0-setup.exe"),
		"-out", filepath.Join(dir, "manifest.json"))
	do(t, "sign", "-key", key, "-manifest", filepath.Join(dir, "manifest.json"), "-out", filepath.Join(dir, "update.json"))
	shown := do(t, "verify", "-pub", printed, "-in", filepath.Join(dir, "update.json"))
	for _, want := range []string{"0.2.0 build 71", "min 60, rollout 100%", "android-arm64-v8a: Spofy-0.2.0-arm64.apk, 204800 bytes", "windows-x64: Spofy-0.2.0-setup.exe, 307200 bytes", "Меньше расход батареи."} {
		if !strings.Contains(shown, want) {
			t.Fatalf("verify shows\n%s\nwithout %q", shown, want)
		}
	}
	ctx := context.Background()
	offer, err := update.Check(ctx, ts.Client(), []string{ts.URL + "/update.json"}, []ed25519.PublicKey{pub}, 65, "android-arm64-v8a", "install-1")
	if err != nil || offer == nil {
		t.Fatalf("offer %+v, err = %v", offer, err)
	}
	if offer.Build != 71 || offer.Version != "0.2.0" || offer.Required || offer.Notes != "Подключение быстрее.\nМеньше расход батареи." {
		t.Fatalf("offer %+v", offer)
	}
	if want := ts.URL + "/out/Spofy-0.2.0-arm64.apk"; !slices.Equal(offer.File.URLs, []string{want, want}) {
		t.Fatalf("urls %v", offer.File.URLs)
	}
	path, err := update.Download(ctx, ts.Client(), offer.File, filepath.Join(dir, "got"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, apk) {
		t.Fatal("downloaded file differs")
	}
}

func TestKeygenWritesTheKeyForTheOwnerOnlyAndPrintsOnlyThePublicKey(t *testing.T) {
	key := filepath.Join(t.TempDir(), "release.key")
	printed := do(t, "keygen", "-out", key)
	fi, err := os.Stat(key)
	if err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600) {
		t.Fatalf("mode %v, err = %v", fi.Mode(), err)
	}
	raw, err := os.ReadFile(key)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		t.Fatalf("key file holds %d bytes, err = %v", len(seed), err)
	}
	pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	if want := base64.StdEncoding.EncodeToString(pub) + "\n"; printed != want {
		t.Fatalf("printed %q, want only %q", printed, want)
	}
}

func TestKeygenRefusesToOverwriteAKey(t *testing.T) {
	key := filepath.Join(t.TempDir(), "release.key")
	write(t, key, []byte("the key we sign with\n"))
	var out bytes.Buffer
	if err := run([]string{"keygen", "-out", key}, &out); err == nil {
		t.Fatal("an existing key was overwritten")
	}
	if got, _ := os.ReadFile(key); string(got) != "the key we sign with\n" || out.Len() != 0 {
		t.Fatalf("key file now %q, printed %q", got, out.String())
	}
}

func TestManifestRefusesWhatWouldReachNoDevice(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "notes.txt"), []byte("Новое"))
	write(t, filepath.Join(dir, "a", "Spofy.apk"), []byte("arm64"))
	write(t, filepath.Join(dir, "b", "Spofy.apk"), []byte("x86_64"))
	common := []string{"manifest", "-version", "0.2.0", "-build", "71", "-notes-file", filepath.Join(dir, "notes.txt"), "-rollout", "100", "-out", filepath.Join(dir, "manifest.json")}
	for name, extra := range map[string][]string{
		"unknown platform":         {"-url-base", "https://u.example/", "-file", "android-arm64=" + filepath.Join(dir, "a", "Spofy.apk")},
		"two files under one name": {"-url-base", "https://u.example/", "-file", "android-arm64-v8a=" + filepath.Join(dir, "a", "Spofy.apk"), "-file", "android-x86_64=" + filepath.Join(dir, "b", "Spofy.apk")},
		"plain http address":       {"-url-base", "http://u.example/", "-file", "android-arm64-v8a=" + filepath.Join(dir, "a", "Spofy.apk")},
		"no address":               {"-file", "android-arm64-v8a=" + filepath.Join(dir, "a", "Spofy.apk")},
	} {
		if err := run(append(slices.Clone(common), extra...), &bytes.Buffer{}); err == nil {
			t.Fatalf("%s: manifest written", name)
		}
		if _, err := os.Stat(filepath.Join(dir, "manifest.json")); !os.IsNotExist(err) {
			t.Fatalf("%s: manifest.json exists", name)
		}
	}
}

func TestVerifyRefusesAnUpdateSignedByAnotherKey(t *testing.T) {
	dir := t.TempDir()
	do(t, "keygen", "-out", filepath.Join(dir, "release.key"))
	other := strings.TrimSpace(do(t, "keygen", "-out", filepath.Join(dir, "other.key")))
	write(t, filepath.Join(dir, "notes.txt"), nil)
	write(t, filepath.Join(dir, "Spofy.apk"), []byte("apk"))
	do(t, "manifest", "-version", "0.2.0", "-build", "71", "-notes-file", filepath.Join(dir, "notes.txt"), "-rollout", "5",
		"-url-base", "https://u.example/", "-file", "android-arm64-v8a="+filepath.Join(dir, "Spofy.apk"), "-out", filepath.Join(dir, "manifest.json"))
	do(t, "sign", "-key", filepath.Join(dir, "release.key"), "-manifest", filepath.Join(dir, "manifest.json"), "-out", filepath.Join(dir, "update.json"))
	err := run([]string{"verify", "-pub", other, "-in", filepath.Join(dir, "update.json")}, &bytes.Buffer{})
	if !errors.Is(err, update.ErrSignature) {
		t.Fatalf("err = %v, want %v", err, update.ErrSignature)
	}
}

func TestMissingCommandOrFlagIsAUsageError(t *testing.T) {
	for _, args := range [][]string{nil, {"publish"}, {"keygen"}, {"sign", "-key", "k", "-manifest", "m"}, {"verify", "-pub", "x", "-in", "f", "extra"}} {
		if err := run(args, &bytes.Buffer{}); !errors.Is(err, errUsage) {
			t.Fatalf("%v: err = %v, want a usage error", args, err)
		}
	}
}
