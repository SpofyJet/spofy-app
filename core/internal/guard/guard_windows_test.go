package guard

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os/exec"
	"strings"
	"testing"

	"github.com/tailscale/wf"
)

func counted(t *testing.T) (kept, dynamic int) {
	t.Helper()
	s, err := wf.New(&wf.Options{Name: "Spofy test"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	subs, err := s.Sublayers()
	if err != nil {
		t.Fatal(err)
	}
	ours := map[wf.SublayerID]bool{}
	for _, sub := range subs {
		if sub.Name == "Spofy" && sub.ID != Kept {
			ours[sub.ID] = true
		}
	}
	all, err := s.Rules()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range all {
		switch {
		case r.Sublayer == Kept:
			kept++
			if !r.Persistent {
				t.Fatalf("rule %s in the kept sublayer is not persistent", r.Name)
			}
		case ours[r.Sublayer]:
			dynamic++
		}
	}
	return kept, dynamic
}

func fresh(t *testing.T) *Windows {
	t.Helper()
	if err := Release(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Release() })
	return &Windows{Local: []netip.Addr{netip.MustParseAddr("172.19.0.1")}}
}

func TestKeptBlockIsLeftByTheProcessAndTakenBackByTheNext(t *testing.T) {
	g := fresh(t)
	if g.Held() {
		t.Fatal("held before anything was raised")
	}
	if err := g.Keep(true); err != nil {
		t.Fatal(err)
	}
	if err := g.Raise(); err != nil {
		t.Fatal(err)
	}
	kept, dynamic := counted(t)
	if kept != 14 || dynamic != 0 {
		t.Fatalf("raised with the kill switch: kept %d, dynamic %d", kept, dynamic)
	}
	if err := g.Raise(); err != nil {
		t.Fatal(err)
	}
	if again, _ := counted(t); again != kept {
		t.Fatalf("raised twice: %d rules instead of %d", again, kept)
	}
	g.Leave()
	if err := g.Lower(); err != nil {
		t.Fatal(err)
	}
	if kept, _ := counted(t); kept != 14 {
		t.Fatalf("a leaving process took the block away: %d rules", kept)
	}
	next := &Windows{Local: g.Local}
	if !next.Held() {
		t.Fatal("the next process does not see the block")
	}
	if err := next.Lower(); err != nil {
		t.Fatal(err)
	}
	if kept, dynamic := counted(t); kept != 0 || dynamic != 0 || next.Held() {
		t.Fatalf("after a disconnect: kept %d, dynamic %d", kept, dynamic)
	}
}

func TestKillSwitchTurnedOnAndOffWhileRaised(t *testing.T) {
	g := fresh(t)
	if err := g.Raise(); err != nil {
		t.Fatal(err)
	}
	if kept, dynamic := counted(t); kept != 0 || dynamic != 14 {
		t.Fatalf("raised without the kill switch: kept %d, dynamic %d", kept, dynamic)
	}
	if err := g.Keep(true); err != nil {
		t.Fatal(err)
	}
	if kept, dynamic := counted(t); kept != 14 || dynamic != 0 {
		t.Fatalf("kill switch on: kept %d, dynamic %d", kept, dynamic)
	}
	if err := g.Keep(false); err != nil {
		t.Fatal(err)
	}
	if kept, dynamic := counted(t); kept != 0 || dynamic != 14 {
		t.Fatalf("kill switch off: kept %d, dynamic %d", kept, dynamic)
	}
	if err := g.Lower(); err != nil {
		t.Fatal(err)
	}
	if err := g.Keep(true); err != nil {
		t.Fatal(err)
	}
	if kept, dynamic := counted(t); kept != 0 || dynamic != 0 {
		t.Fatalf("kill switch on while lowered raised something: kept %d, dynamic %d", kept, dynamic)
	}
}

func TestReleaseTakesAwayABlockLeftBehind(t *testing.T) {
	g := fresh(t)
	g.Keep(true)
	if err := g.Raise(); err != nil {
		t.Fatal(err)
	}
	g.Leave()
	g.Lower()
	if err := Release(); err != nil {
		t.Fatal(err)
	}
	if kept, _ := counted(t); kept != 0 || (&Windows{}).Held() {
		t.Fatalf("after release: kept %d", kept)
	}
	if err := Release(); err != nil {
		t.Fatalf("release with nothing left: %v", err)
	}
}

func curl(t *testing.T, address string) string {
	t.Helper()
	out, _ := exec.Command("curl.exe", "-s", "-m", "4", "-o", "NUL", "-w", "%{http_code}", address).Output()
	return strings.TrimSpace(string(out))
}

func TestAnotherProgramReachesLoopbackButNotTheOutside(t *testing.T) {
	g := fresh(t)
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer local.Close()
	for _, keep := range []bool{false, true} {
		if err := g.Keep(keep); err != nil {
			t.Fatal(err)
		}
		if err := g.Raise(); err != nil {
			t.Fatal(err)
		}
		if code := curl(t, local.URL); code != "204" {
			t.Fatalf("kill switch %v: loopback of another program got %q", keep, code)
		}
		if code := curl(t, "http://1.1.1.1/"); code != "000" {
			t.Fatalf("kill switch %v: another program went outside with %q", keep, code)
		}
		if err := g.Lower(); err != nil {
			t.Fatal(err)
		}
	}
}
