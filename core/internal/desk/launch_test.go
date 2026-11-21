package desk

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestAnInvitationIsTakenOnceFreshAndOnlyForSpofy(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	runs := []Run{{1, "spofy-tray.exe"}, {2, "spofy-tray.exe"}, {1, "spofy.exe"}, {1, "spofy-tray.exe"}, {0, "spofy.exe"}, {1, "cmd.exe"}, {1, `..\spofy.exe`}}
	if err := Invite(root, runs, now); err != nil {
		t.Fatal(err)
	}
	want := []Run{{1, "spofy-tray.exe"}, {2, "spofy-tray.exe"}, {1, "spofy.exe"}}
	if got := Invited(root, now.Add(time.Minute)); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := Invited(root, now.Add(time.Minute)); got != nil {
		t.Fatalf("an invitation was taken twice: %v", got)
	}
	for name, at := range map[string]time.Time{"stale": now.Add(-3 * time.Minute), "from the future": now.Add(2 * time.Minute)} {
		Invite(root, runs, at)
		if got := Invited(root, now); got != nil {
			t.Fatalf("%s invitation was taken: %v", name, got)
		}
		if _, err := os.Stat(filepath.Join(root, invitation)); !os.IsNotExist(err) {
			t.Fatalf("%s invitation stays: %v", name, err)
		}
	}
	os.WriteFile(filepath.Join(root, invitation), []byte("{"), 0o600)
	if got := Invited(root, now); got != nil {
		t.Fatalf("a broken invitation was taken: %v", got)
	}
	Invite(root, runs, now)
	Uninvite(root)
	if got := Invited(root, now); got != nil {
		t.Fatalf("a withdrawn invitation was taken: %v", got)
	}
}
