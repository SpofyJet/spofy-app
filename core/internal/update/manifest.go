package update

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrUnreachable = errors.New("update: no source gave an update file")
	ErrSignature   = errors.New("update: not signed by a trusted key")
	ErrManifest    = errors.New("update: manifest is not valid")
)

const (
	maxEnvelope = 64 << 10
	maxNotes    = 2000
	maxSize     = 200 << 20
)

var (
	versionRe  = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+-]{0,31}$`)
	platformRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)
	nameRe     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
	hashRe     = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

var platforms = map[string]string{
	"android/arm64": "android-arm64-v8a",
	"android/arm":   "android-armeabi-v7a",
	"android/amd64": "android-x86_64",
	"windows/amd64": "windows-x64",
}

func Platform(goos, goarch string) string {
	return platforms[goos+"/"+goarch]
}

func Platforms() []string {
	return slices.Sorted(maps.Values(platforms))
}

type Manifest struct {
	Version  string          `json:"version"`
	Build    int             `json:"build"`
	Released time.Time       `json:"released"`
	Notes    string          `json:"notes"`
	Min      int             `json:"min"`
	Rollout  int             `json:"rollout"`
	Files    map[string]File `json:"files"`
}

type File struct {
	Name   string   `json:"name"`
	Size   int64    `json:"size"`
	SHA256 string   `json:"sha256"`
	URLs   []string `json:"urls"`
}

type envelope struct {
	Manifest  []byte `json:"manifest"`
	Signature []byte `json:"signature"`
}

func Open(data []byte, keys []ed25519.PublicKey) (*Manifest, error) {
	var e envelope
	if err := strict(data, &e); err != nil {
		return nil, fmt.Errorf("update: not an update file: %w", err)
	}
	if len(e.Manifest) == 0 || len(e.Signature) == 0 {
		return nil, errors.New("update: not an update file: manifest or signature is empty")
	}
	if !slices.ContainsFunc(keys, func(k ed25519.PublicKey) bool {
		return len(k) == ed25519.PublicKeySize && ed25519.Verify(k, e.Manifest, e.Signature)
	}) {
		return nil, ErrSignature
	}
	return ParseManifest(e.Manifest)
}

func Seal(manifest []byte, key ed25519.PrivateKey) ([]byte, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("update: not an ed25519 private key")
	}
	if _, err := ParseManifest(manifest); err != nil {
		return nil, err
	}
	b, err := json.Marshal(envelope{Manifest: manifest, Signature: ed25519.Sign(key, manifest)})
	if err != nil {
		return nil, err
	}
	if len(b) > maxEnvelope {
		return nil, fmt.Errorf("update: update file is %d bytes, the app reads at most %d", len(b), maxEnvelope)
	}
	return b, nil
}

func ParseManifest(b []byte) (*Manifest, error) {
	var m Manifest
	if err := strict(b, &m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	if err := m.valid(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	return &m, nil
}

func strict(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("data after the object")
	}
	return nil
}

func (m *Manifest) valid() error {
	switch {
	case m.Build <= 0:
		return fmt.Errorf("build %d", m.Build)
	case !versionRe.MatchString(m.Version):
		return fmt.Errorf("version %q", m.Version)
	case m.Released.IsZero():
		return errors.New("no release time")
	case utf8.RuneCountInString(m.Notes) > maxNotes:
		return fmt.Errorf("notes are over %d characters", maxNotes)
	case m.Min < 0 || m.Min > m.Build:
		return fmt.Errorf("min %d with build %d", m.Min, m.Build)
	case m.Rollout < 0 || m.Rollout > 100:
		return fmt.Errorf("rollout %d", m.Rollout)
	case len(m.Files) == 0:
		return errors.New("no files")
	}
	for _, p := range slices.Sorted(maps.Keys(m.Files)) {
		if !platformRe.MatchString(p) {
			return fmt.Errorf("platform %q", p)
		}
		if err := m.Files[p].valid(); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	return nil
}

func (f File) valid() error {
	switch {
	case !nameRe.MatchString(f.Name) || strings.Contains(f.Name, ".."):
		return fmt.Errorf("name %q", f.Name)
	case f.Size <= 0 || f.Size > maxSize:
		return fmt.Errorf("size %d", f.Size)
	case !hashRe.MatchString(f.SHA256):
		return fmt.Errorf("sha256 %q", f.SHA256)
	case len(f.URLs) == 0:
		return errors.New("no urls")
	}
	for _, s := range f.URLs {
		u, err := url.Parse(s)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return fmt.Errorf("url %q", s)
		}
	}
	return nil
}
