package main

import (
	"runtime"
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
