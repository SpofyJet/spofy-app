package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func newKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func sample() Manifest {
	return Manifest{
		Version:  "0.2.0",
		Build:    71,
		Released: time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC),
		Notes:    "Подключение быстрее, меньше расход батареи.",
		Min:      60,
		Rollout:  100,
		Files: map[string]File{
			"android-arm64-v8a": {
				Name:   "Spofy-0.2.0-arm64.apk",
				Size:   18716344,
				SHA256: strings.Repeat("ab", 32),
				URLs:   []string{"https://a.example/u/Spofy-0.2.0-arm64.apk", "https://b.example/u/Spofy-0.2.0-arm64.apk"},
			},
			"windows-x64": {
				Name:   "Spofy-0.2.0-setup.exe",
				Size:   25000000,
				SHA256: strings.Repeat("cd", 32),
				URLs:   []string{"https://a.example/u/Spofy-0.2.0-setup.exe"},
			},
		},
	}
}

func encode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sealed(t *testing.T, m Manifest, priv ed25519.PrivateKey) []byte {
	t.Helper()
	b, err := Seal(encode(t, m), priv)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func signedRaw(t *testing.T, manifest []byte, priv ed25519.PrivateKey) []byte {
	t.Helper()
	return encode(t, envelope{Manifest: manifest, Signature: ed25519.Sign(priv, manifest)})
}

func TestASealedManifestOpensWithItsKeyAsItWasWritten(t *testing.T) {
	pub, priv := newKey(t)
	m, err := Open(sealed(t, sample(), priv), []ed25519.PublicKey{pub})
	if err != nil {
		t.Fatal(err)
	}
	want := sample()
	if string(encode(t, m)) != string(encode(t, want)) {
		t.Fatalf("opened %+v, want %+v", m, want)
	}
}

func TestOpenRefusesAManifestWithOneChangedByte(t *testing.T) {
	pub, priv := newKey(t)
	manifest := encode(t, sample())
	sig := ed25519.Sign(priv, manifest)
	changed := []byte(strings.Replace(string(manifest), `"build":71`, `"build":72`, 1))
	if string(changed) == string(manifest) {
		t.Fatal("the manifest did not change")
	}
	_, err := Open(encode(t, envelope{Manifest: changed, Signature: sig}), []ed25519.PublicKey{pub})
	if !errors.Is(err, ErrSignature) {
		t.Fatalf("err = %v, want %v", err, ErrSignature)
	}
}

func TestOpenRefusesAChangedSignature(t *testing.T) {
	pub, priv := newKey(t)
	manifest := encode(t, sample())
	for _, sig := range [][]byte{
		func() []byte { s := ed25519.Sign(priv, manifest); s[10] ^= 1; return s }(),
		ed25519.Sign(priv, manifest)[:ed25519.SignatureSize-1],
		append(ed25519.Sign(priv, manifest), 0),
	} {
		_, err := Open(encode(t, envelope{Manifest: manifest, Signature: sig}), []ed25519.PublicKey{pub})
		if !errors.Is(err, ErrSignature) {
			t.Fatalf("signature of %d bytes: err = %v, want %v", len(sig), err, ErrSignature)
		}
	}
}

func TestOpenRefusesAManifestSignedByAnUnknownKey(t *testing.T) {
	pub, _ := newKey(t)
	_, other := newKey(t)
	_, err := Open(sealed(t, sample(), other), []ed25519.PublicKey{pub})
	if !errors.Is(err, ErrSignature) {
		t.Fatalf("err = %v, want %v", err, ErrSignature)
	}
}

func TestOpenAcceptsAnyPinnedKeySoKeysCanBeRotated(t *testing.T) {
	old, _ := newKey(t)
	next, priv := newKey(t)
	if _, err := Open(sealed(t, sample(), priv), []ed25519.PublicKey{old, next}); err != nil {
		t.Fatal(err)
	}
}

func TestAMalformedPinnedKeyIsSkippedNotPanickedOn(t *testing.T) {
	pub, priv := newKey(t)
	if _, err := Open(sealed(t, sample(), priv), []ed25519.PublicKey{pub[:5], nil, pub}); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(sealed(t, sample(), priv), []ed25519.PublicKey{pub[:5]}); !errors.Is(err, ErrSignature) {
		t.Fatalf("err = %v, want %v", err, ErrSignature)
	}
}

func TestStrictJSONRefusesUnknownFieldsAndTrailingData(t *testing.T) {
	pub, priv := newKey(t)
	manifest := encode(t, sample())
	extra := []byte(strings.Replace(string(manifest), `"build":71`, `"build":71,"channel":"beta"`, 1))
	inFile := []byte(strings.Replace(string(manifest), `"size":18716344`, `"size":18716344,"mode":"0755"`, 1))
	for name, b := range map[string][]byte{
		"unknown field":         extra,
		"unknown field in file": inFile,
		"trailing data":         append(append([]byte{}, manifest...), []byte(` {}`)...),
	} {
		if _, err := Open(signedRaw(t, b, priv), []ed25519.PublicKey{pub}); !errors.Is(err, ErrManifest) {
			t.Fatalf("%s: err = %v, want %v", name, err, ErrManifest)
		}
	}
	env := strings.Replace(string(signedRaw(t, manifest, priv)), `{`, `{"key":"x",`, 1)
	if _, err := Open([]byte(env), []ed25519.PublicKey{pub}); err == nil || errors.Is(err, ErrSignature) || errors.Is(err, ErrManifest) {
		t.Fatalf("envelope with an unknown field: err = %v", err)
	}
}

func TestOpenRefusesWhatIsNotAnUpdateFile(t *testing.T) {
	pub, _ := newKey(t)
	for _, b := range []string{"", "null", "{}", "[]", "<html>blocked</html>", `{"manifest":"!!","signature":""}`} {
		_, err := Open([]byte(b), []ed25519.PublicKey{pub})
		if err == nil || errors.Is(err, ErrSignature) || errors.Is(err, ErrManifest) {
			t.Fatalf("%q: err = %v", b, err)
		}
	}
}

func TestEveryBrokenManifestIsRefused(t *testing.T) {
	file := func(edit func(*File)) func(*Manifest) {
		return func(m *Manifest) {
			f := m.Files["android-arm64-v8a"]
			f.URLs = append([]string{}, f.URLs...)
			edit(&f)
			m.Files["android-arm64-v8a"] = f
		}
	}
	for name, edit := range map[string]func(*Manifest){
		"zero build":            func(m *Manifest) { m.Build = 0 },
		"negative build":        func(m *Manifest) { m.Build = -1 },
		"no version":            func(m *Manifest) { m.Version = "" },
		"long version":          func(m *Manifest) { m.Version = strings.Repeat("1", 33) },
		"version with a space":  func(m *Manifest) { m.Version = "0.2 beta" },
		"no release time":       func(m *Manifest) { m.Released = time.Time{} },
		"notes over 2000":       func(m *Manifest) { m.Notes = strings.Repeat("ж", 2001) },
		"min above build":       func(m *Manifest) { m.Min = 72 },
		"negative min":          func(m *Manifest) { m.Min = -1 },
		"rollout over 100":      func(m *Manifest) { m.Rollout = 101 },
		"negative rollout":      func(m *Manifest) { m.Rollout = -1 },
		"no files":              func(m *Manifest) { m.Files = nil },
		"odd platform":          func(m *Manifest) { m.Files["Android ARM"] = m.Files["windows-x64"] },
		"empty size":            file(func(f *File) { f.Size = 0 }),
		"size over 200 MB":      file(func(f *File) { f.Size = 200<<20 + 1 }),
		"short sha256":          file(func(f *File) { f.SHA256 = f.SHA256[:63] }),
		"upper case sha256":     file(func(f *File) { f.SHA256 = strings.ToUpper(f.SHA256) }),
		"sha256 not hex":        file(func(f *File) { f.SHA256 = strings.Repeat("zz", 32) }),
		"no urls":               file(func(f *File) { f.URLs = nil }),
		"plain http url":        file(func(f *File) { f.URLs[1] = "http://b.example/u/Spofy-0.2.0-arm64.apk" }),
		"url without host":      file(func(f *File) { f.URLs[0] = "https:///Spofy.apk" }),
		"url with a password":   file(func(f *File) { f.URLs[0] = "https://user:pass@a.example/Spofy.apk" }),
		"relative url":          file(func(f *File) { f.URLs[0] = "/u/Spofy.apk" }),
		"name with a folder":    file(func(f *File) { f.Name = "u/Spofy.apk" }),
		"name going up":         file(func(f *File) { f.Name = "../Spofy.apk" }),
		"name with two dots":    file(func(f *File) { f.Name = "Spofy..apk" }),
		"name with a backslash": file(func(f *File) { f.Name = `u\Spofy.apk` }),
		"hidden name":           file(func(f *File) { f.Name = ".Spofy.apk" }),
		"no name":               file(func(f *File) { f.Name = "" }),
	} {
		m := sample()
		edit(&m)
		if _, err := ParseManifest(encode(t, m)); !errors.Is(err, ErrManifest) {
			t.Errorf("%s: err = %v, want %v", name, err, ErrManifest)
		}
	}
}

func TestTheLongestAllowedNotesInRussianPass(t *testing.T) {
	m := sample()
	m.Notes = strings.Repeat("ж", 2000)
	if _, err := ParseManifest(encode(t, m)); err != nil {
		t.Fatal(err)
	}
}

func TestSealRefusesAManifestTheAppWouldRefuse(t *testing.T) {
	_, priv := newKey(t)
	m := sample()
	m.Rollout = 101
	if _, err := Seal(encode(t, m), priv); !errors.Is(err, ErrManifest) {
		t.Fatalf("err = %v, want %v", err, ErrManifest)
	}
	if _, err := Seal(encode(t, sample()), priv[:10]); err == nil {
		t.Fatal("a short private key was accepted")
	}
}

func TestSealRefusesAnUpdateFileTooBigForTheApp(t *testing.T) {
	_, priv := newKey(t)
	m := sample()
	f := m.Files["windows-x64"]
	for i := range 1500 {
		f.URLs = append(f.URLs, fmt.Sprintf("https://mirror%d.example/updates/%s", i, f.Name))
	}
	m.Files["windows-x64"] = f
	if _, err := Seal(encode(t, m), priv); err == nil {
		t.Fatal("an update file over 64 KB was sealed")
	}
}

func TestPlatformNamesTheFilesWeBuild(t *testing.T) {
	for _, c := range []struct{ goos, goarch, want string }{
		{"android", "arm64", "android-arm64-v8a"},
		{"android", "arm", "android-armeabi-v7a"},
		{"android", "amd64", "android-x86_64"},
		{"windows", "amd64", "windows-x64"},
		{"windows", "arm64", ""},
		{"linux", "amd64", ""},
	} {
		if got := Platform(c.goos, c.goarch); got != c.want {
			t.Errorf("Platform(%s, %s) = %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
	if got := strings.Join(Platforms(), " "); got != "android-arm64-v8a android-armeabi-v7a android-x86_64 windows-x64" {
		t.Fatalf("Platforms() = %s", got)
	}
}
