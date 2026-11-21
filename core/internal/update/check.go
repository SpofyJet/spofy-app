package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var sourceTimeout = 15 * time.Second

type Offer struct {
	Version  string
	Build    int
	Notes    string
	Required bool
	File     File
}

func Check(ctx context.Context, c *http.Client, sources []string, keys []ed25519.PublicKey, current int, platform, install string) (*Offer, error) {
	worst := ErrUnreachable
	var errs tried
	for _, src := range sources {
		m, err := fetch(ctx, c, src, keys)
		if err == nil {
			return m.offer(current, platform, install), nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		switch {
		case errors.Is(err, ErrSignature):
			worst = ErrSignature
		case errors.Is(err, ErrManifest) && worst == ErrUnreachable:
			worst = ErrManifest
		}
		errs = append(errs, fmt.Errorf("%s: %w", src, err))
	}
	return nil, fmt.Errorf("%w: %v", worst, errs)
}

func fetch(ctx context.Context, c *http.Client, src string, keys []ed25519.PublicKey) (*Manifest, error) {
	ctx, cancel := context.WithTimeout(ctx, sourceTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return nil, bare(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxEnvelope+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxEnvelope {
		return nil, fmt.Errorf("answer is over %d bytes", maxEnvelope)
	}
	return Open(b, keys)
}

func (m *Manifest) offer(current int, platform, install string) *Offer {
	f, ok := m.Files[platform]
	required := current < m.Min
	if m.Build <= current || !ok || !required && bucket(install, m.Build) >= m.Rollout {
		return nil
	}
	return &Offer{Version: m.Version, Build: m.Build, Notes: m.Notes, Required: required, File: f}
}

func bucket(install string, build int) int {
	sum := sha256.Sum256([]byte(install + ":" + strconv.Itoa(build)))
	return int(binary.BigEndian.Uint64(sum[:8]) % 100)
}

func bare(err error) error {
	if ue, ok := err.(*url.Error); ok {
		return ue.Err
	}
	return err
}

type tried []error

func (t tried) Error() string {
	if len(t) == 0 {
		return "no sources"
	}
	s := make([]string, len(t))
	for i, err := range t {
		s[i] = err.Error()
	}
	return strings.Join(s, "; ")
}

func (t tried) Unwrap() []error {
	return t
}
