package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"runtime"
	"slices"
	"testing"
)

func TestTheReleaseKeysAndBuildAreReadable(t *testing.T) {
	if n := len(publicKeys()); n != len(updateKeys) || n < 2 {
		t.Fatalf("%d of %d keys decode", n, len(updateKeys))
	}
	for _, b := range []struct {
		text string
		want int
	}{{"0", 0}, {"7", 7}, {"dev", 0}, {"-3", 0}} {
		build = b.text
		if got := buildNumber(); got != b.want {
			t.Fatalf("%q -> %d", b.text, got)
		}
	}
	build = "0"
	u := updates(t.TempDir())
	if len(u.Sources) == 0 || u.Every <= 0 || (runtime.GOOS == "linux" && u.Platform != "") {
		t.Fatalf("%+v", u)
	}
}

func TestAStandGivesItsOwnUpdateSourceAndKeyOrNothingChanges(t *testing.T) {
	public, _, _ := ed25519.GenerateKey(nil)
	key := base64.StdEncoding.EncodeToString(public)
	for _, c := range []struct {
		source, key string
		own         bool
	}{
		{"https://10.0.2.2:8443/update.json", key, true},
		{"http://10.0.2.2/update.json", key, false},
		{"https://10.0.2.2:8443/update.json", "bm90IGEga2V5", false},
		{"", key, false},
		{"https://10.0.2.2:8443/update.json", "", false},
	} {
		t.Setenv("SPOFY_UPDATE_SOURCE", c.source)
		t.Setenv("SPOFY_UPDATE_KEY", c.key)
		u := updates(t.TempDir())
		own := slices.Equal(u.Sources, []string{c.source}) && len(u.Keys) == 1 && u.Keys[0].Equal(public)
		builtin := slices.Equal(u.Sources, updateSources) && len(u.Keys) == len(updateKeys)
		if own != c.own || own == builtin {
			t.Fatalf("%q %q: sources %v, %d keys", c.source, c.key, u.Sources, len(u.Keys))
		}
	}
}
