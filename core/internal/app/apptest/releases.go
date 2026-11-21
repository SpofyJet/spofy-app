package apptest

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/update"
)

const (
	ReleasesHost = "updates.example.test"
	ReleasesURL  = "https://" + ReleasesHost + "/update.json"
	Platform     = "android-arm64-v8a"
)

type Releases struct {
	t    testing.TB
	Key  ed25519.PrivateKey
	mu   sync.Mutex
	env  []byte
	file []byte
}

func NewReleases(t testing.TB, p *Panel) *Releases {
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 7
	r := &Releases{t: t, Key: ed25519.NewKeyFromSeed(seed)}
	if p != nil {
		p.Route(ReleasesHost, r)
	}
	return r
}

func (r *Releases) Public() ed25519.PublicKey {
	return r.Key.Public().(ed25519.PublicKey)
}

func (r *Releases) Publish(build, rollout int, file, served []byte) {
	r.t.Helper()
	sum := sha256.Sum256(file)
	name := fmt.Sprintf("Spofy-0.%d.0.apk", build)
	m := update.Manifest{
		Version: fmt.Sprintf("0.%d.0", build), Build: build, Released: time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC),
		Notes: "Что нового", Rollout: rollout,
		Files: map[string]update.File{Platform: {Name: name, Size: int64(len(file)), SHA256: hex.EncodeToString(sum[:]), URLs: []string{"https://" + ReleasesHost + "/f/" + name}}},
	}
	b, err := json.Marshal(m)
	if err != nil {
		r.t.Fatal(err)
	}
	env, err := update.Seal(b, r.Key)
	if err != nil {
		r.t.Fatal(err)
	}
	r.mu.Lock()
	r.env, r.file = env, served
	r.mu.Unlock()
}

func (r *Releases) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	env, file := r.env, r.file
	r.mu.Unlock()
	if req.URL.Path == "/update.json" {
		if env == nil {
			http.NotFound(w, req)
			return
		}
		w.Write(env)
		return
	}
	w.Header().Set("Content-Length", fmt.Sprint(len(file)))
	w.Write(file)
}
