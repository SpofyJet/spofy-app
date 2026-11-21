package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/app"
	"github.com/SpofyJet/spofy-app/core/internal/update"
)

var (
	build     = "0"
	engineRev = "dev"
)

var updateSources = []string{"https://github.com/SpofyJet/spofy-app/releases/latest/download/update.json"}

var updateKeys = []string{"HS59BMjW6LqywL7rb4f/wnz0lQWMDI9mJ2Pd68CZprA=", "UjlTisloQPNI5/M3Pg2gFISpEb5Y8kpmvtg0IhOMInI="}

func buildNumber() int {
	n, err := strconv.Atoi(build)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func publicKey(text string) ed25519.PublicKey {
	if b, err := base64.StdEncoding.DecodeString(text); err == nil && len(b) == ed25519.PublicKeySize {
		return ed25519.PublicKey(b)
	}
	return nil
}

func publicKeys() []ed25519.PublicKey {
	var keys []ed25519.PublicKey
	for _, k := range updateKeys {
		if key := publicKey(k); key != nil {
			keys = append(keys, key)
		}
	}
	return keys
}

func updates(folder string) app.Updates {
	sources, keys := updateSources, publicKeys()
	if source, key := os.Getenv("SPOFY_UPDATE_SOURCE"), publicKey(os.Getenv("SPOFY_UPDATE_KEY")); strings.HasPrefix(source, "https://") && key != nil {
		sources, keys = []string{source}, []ed25519.PublicKey{key}
	}
	return app.Updates{
		Sources:  sources,
		Keys:     keys,
		Platform: update.Platform(runtime.GOOS, runtime.GOARCH),
		Dir:      filepath.Join(folder, "updates"),
		First:    5 * time.Second,
		Every:    6 * time.Hour,
	}
}
