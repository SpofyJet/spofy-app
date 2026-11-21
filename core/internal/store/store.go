package store

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/sub"
)

const Version = 1

const (
	fileMain  = "state"
	fileFresh = "state.new"
	fileOld   = "state.old"
	lostMark  = ".lost"
)

var (
	ErrKey        = errors.New("store: key must be 32 bytes")
	ErrUnreadable = errors.New("store: saved data cannot be read")
	ErrNewer      = errors.New("store: saved data belongs to a newer version")
)

var magic = []byte("SPS1")

type Delay struct {
	Ms int64     `json:"ms"`
	At time.Time `json:"at"`
}

type State struct {
	Version   int                        `json:"version"`
	ID        string                     `json:"id,omitempty"`
	URL       string                     `json:"url,omitempty"`
	Profile   *sub.Profile               `json:"profile,omitempty"`
	FetchedAt time.Time                  `json:"fetchedAt,omitzero"`
	Server    string                     `json:"server,omitempty"`
	Settings  map[string]json.RawMessage `json:"settings,omitempty"`
	Delays    map[string]Delay           `json:"delays,omitempty"`
	Others    []Saved                    `json:"others,omitempty"`
}

type Saved struct {
	ID        string           `json:"id,omitempty"`
	URL       string           `json:"url,omitempty"`
	Profile   *sub.Profile     `json:"profile"`
	FetchedAt time.Time        `json:"fetchedAt,omitzero"`
	Server    string           `json:"server,omitempty"`
	Delays    map[string]Delay `json:"delays,omitempty"`
}

type Store struct {
	dir  string
	aead cipher.AEAD

	mu     sync.Mutex
	raw    []byte
	intact bool
	source string
}

func Open(dir string, key []byte) (*Store, error) {
	if len(key) != 32 {
		return nil, ErrKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	os.Chmod(dir, 0o700)
	s := &Store{dir: dir, aead: aead}
	found, newer := false, false
	for _, name := range []string{fileMain, fileFresh, fileOld} {
		data, err := os.ReadFile(s.path(name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		found = true
		if err != nil {
			continue
		}
		raw, err := s.unseal(data)
		if errors.Is(err, ErrNewer) {
			newer = true
		}
		if err != nil {
			continue
		}
		s.raw, s.source = raw, name
		switch name {
		case fileMain:
			s.intact = true
			os.Remove(s.path(fileFresh))
		case fileFresh:
			if os.Rename(s.path(fileFresh), s.path(fileMain)) == nil {
				s.intact = true
				syncDir(dir)
			}
		}
		return s, nil
	}
	switch {
	case newer:
		return nil, ErrNewer
	case found:
		return nil, ErrUnreadable
	}
	s.raw, _ = json.Marshal(State{Version: Version})
	return s, nil
}

func Reset(dir string) error {
	for _, name := range []string{fileMain, fileFresh, fileOld} {
		from := filepath.Join(dir, name)
		if err := os.Rename(from, from+lostMark); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	syncDir(dir)
	return nil
}

func (s *Store) Recovered() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.source != "" && s.source != fileMain
}

func (s *Store) Get() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	var st State
	json.Unmarshal(s.raw, &st)
	return st
}

func (s *Store) Update(change func(*State)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var next State
	if err := json.Unmarshal(s.raw, &next); err != nil {
		return err
	}
	change(&next)
	next.Version = Version
	raw, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if bytes.Equal(raw, s.raw) {
		return nil
	}
	if err := s.save(raw); err != nil {
		return err
	}
	s.raw = raw
	return nil
}

func (s *Store) path(name string) string {
	return filepath.Join(s.dir, name)
}

func (s *Store) unseal(data []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(data) < len(magic)+n || !bytes.Equal(data[:len(magic)], magic) {
		return nil, ErrUnreadable
	}
	raw, err := s.aead.Open(nil, data[len(magic):len(magic)+n], data[len(magic)+n:], magic)
	if err != nil {
		return nil, ErrUnreadable
	}
	var head struct {
		Version int `json:"version"`
	}
	if json.Unmarshal(raw, &head) != nil || head.Version < 1 {
		return nil, ErrUnreadable
	}
	if head.Version > Version {
		return nil, ErrNewer
	}
	return raw, nil
}

func (s *Store) save(raw []byte) error {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	out := append(append([]byte{}, magic...), nonce...)
	out = s.aead.Seal(out, nonce, raw, magic)
	if err := writeSync(s.path(fileFresh), out); err != nil {
		return err
	}
	if s.intact {
		if err := os.Rename(s.path(fileMain), s.path(fileOld)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if err := os.Rename(s.path(fileFresh), s.path(fileMain)); err != nil {
		return err
	}
	s.intact, s.source = true, fileMain
	syncDir(s.dir)
	return nil
}

func writeSync(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
}
