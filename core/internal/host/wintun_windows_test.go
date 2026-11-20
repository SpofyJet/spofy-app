package host

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/xtls/xray-core/proxy/tun"
	"golang.zx2c4.com/wintun"
)

const testAdapter = "spofytest0"

func TestWintunHelper(t *testing.T) {
	role := os.Getenv("SPOFY_TEST_WINTUN")
	if role == "" {
		t.Skip("runs only as a helper process")
	}
	var a *wintun.Adapter
	var err error
	if role == "create" {
		a, err = wintun.CreateAdapter(testAdapter, "Spofy", tun.AdapterGUID(testAdapter))
	} else {
		a, err = wintun.OpenAdapter(testAdapter)
	}
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	var s wintun.Session
	if role == "borrow" {
		if s, err = a.StartSession(0x20000); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
	fmt.Println("ready")
	time.Sleep(time.Minute)
	if role == "borrow" {
		s.End()
	}
	a.Close()
	os.Exit(0)
}

func helper(t *testing.T, role string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestWintunHelper$")
	cmd.Env = append(os.Environ(), "SPOFY_TEST_WINTUN="+role)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stop(cmd) })
	got := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(out).ReadString('\n')
		got <- strings.TrimSpace(line)
	}()
	select {
	case line := <-got:
		if line != "ready" {
			t.Fatalf("the %s helper: %q", role, line)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("the %s helper did not start", role)
	}
	return cmd
}

func stop(cmd *exec.Cmd) {
	cmd.Process.Kill()
	cmd.Wait()
}

func within(t *testing.T, what string, limit time.Duration, f func()) {
	t.Helper()
	began := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
		t.Logf("%s: %v", what, time.Since(began).Round(time.Millisecond))
	case <-time.After(limit):
		t.Fatalf("%s did not return in %v", what, limit)
	}
}

func present(name string) bool {
	a, err := wintun.OpenAdapter(name)
	if err != nil {
		return false
	}
	a.Close()
	return true
}

func gone(name string, limit time.Duration) bool {
	for end := time.Now().Add(limit); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		if !present(name) {
			return true
		}
	}
	return !present(name)
}

func clean(t *testing.T) {
	t.Helper()
	if !gone(testAdapter, leftover) {
		t.Fatal("an adapter is left from before")
	}
}

func TestTheAdapterLivesAsLongAsItIsRaised(t *testing.T) {
	clean(t)
	w := &Wintun{Name: testAdapter}
	var err error
	within(t, "raise", time.Minute, func() { err = w.Raise() })
	if err != nil {
		t.Fatal(err)
	}
	within(t, "raise again", time.Second, func() { err = w.Raise() })
	if err != nil {
		t.Fatalf("raised again: %v", err)
	}
	if !present(testAdapter) {
		w.Lower()
		t.Fatal("no adapter while raised")
	}
	within(t, "lower", 30*time.Second, func() { w.Lower() })
	if !gone(testAdapter, leftover) {
		t.Fatal("the adapter outlived Lower")
	}
	within(t, "lower again", time.Second, func() { err = w.Lower() })
	if err != nil {
		t.Fatalf("lowered again: %v", err)
	}
}

func TestAKilledBorrowerLeavesTheAdapterReadyForTheNext(t *testing.T) {
	clean(t)
	w := &Wintun{Name: testAdapter}
	var err error
	within(t, "raise", time.Minute, func() { err = w.Raise() })
	if err != nil {
		t.Fatal(err)
	}
	defer within(t, "lower", 30*time.Second, func() { w.Lower() })
	for round := range 5 {
		borrower := helper(t, "borrow")
		within(t, fmt.Sprintf("round %d: kill the borrower", round), 30*time.Second, func() { stop(borrower) })
		var a *wintun.Adapter
		within(t, fmt.Sprintf("round %d: open", round), 30*time.Second, func() { a, err = wintun.OpenAdapter(testAdapter) })
		if err != nil {
			t.Fatalf("round %d: the adapter went with the killed borrower: %v", round, err)
		}
		var s wintun.Session
		within(t, fmt.Sprintf("round %d: start a session", round), 30*time.Second, func() { s, err = a.StartSession(0x20000) })
		if err != nil {
			a.Close()
			t.Fatalf("round %d: the killed borrower still holds the session: %v", round, err)
		}
		within(t, fmt.Sprintf("round %d: end the session and close", round), 30*time.Second, func() {
			s.End()
			a.Close()
		})
	}
}

func TestTheAdapterOfAKilledOwnerMakesWayForANewOne(t *testing.T) {
	for round := range 3 {
		clean(t)
		owner := helper(t, "create")
		within(t, fmt.Sprintf("round %d: kill the owner", round), 30*time.Second, func() { stop(owner) })
		w := &Wintun{Name: testAdapter}
		var err error
		within(t, fmt.Sprintf("round %d: raise", round), time.Minute, func() { err = w.Raise() })
		if err != nil {
			began := time.Now()
			later := gone(testAdapter, 30*time.Second)
			t.Fatalf("round %d: %v; gone %v after %v more", round, err, later, time.Since(began).Round(time.Millisecond))
		}
		time.Sleep(2 * time.Second)
		alive := present(testAdapter)
		within(t, fmt.Sprintf("round %d: lower", round), 30*time.Second, func() { w.Lower() })
		if !alive {
			t.Fatalf("round %d: the new adapter went away with the old one", round)
		}
	}
}

func TestTheAdapterOfALiveProgramIsLeftAlone(t *testing.T) {
	clean(t)
	other := helper(t, "create")
	w := &Wintun{Name: testAdapter}
	var err error
	within(t, "raise", time.Minute, func() { err = w.Raise() })
	within(t, "lower", 30*time.Second, func() { w.Lower() })
	alive := present(testAdapter)
	within(t, "kill the owner", 30*time.Second, func() { stop(other) })
	if err == nil {
		t.Fatal("raised over the adapter of a live program")
	}
	if !alive {
		t.Fatal("the adapter of the live program is gone")
	}
}
