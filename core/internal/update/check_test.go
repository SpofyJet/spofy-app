package update

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

const arm64 = "android-arm64-v8a"

func serve(t *testing.T, h http.HandlerFunc) string {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts.URL + "/update.json"
}

func answer(body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { w.Write(body) }
}

func refused(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return "https://" + addr + "/update.json"
}

func check(sources []string, keys []ed25519.PublicKey, current int, install string) (*Offer, error) {
	return Check(context.Background(), &http.Client{}, sources, keys, current, arm64, install)
}

func installIn(t *testing.T, build int, inside bool, rollout int) string {
	t.Helper()
	for i := range 1000 {
		id := "install-" + strconv.Itoa(i)
		if bucket(id, build) < rollout == inside {
			return id
		}
	}
	t.Fatal("no install found")
	return ""
}

func TestCheckOffersANewerBuildSignedByATrustedKey(t *testing.T) {
	pub, priv := newKey(t)
	o, err := check([]string{serve(t, answer(sealed(t, sample(), priv)))}, []ed25519.PublicKey{pub}, 65, "install-1")
	if err != nil {
		t.Fatal(err)
	}
	m := sample()
	want := &Offer{Version: "0.2.0", Build: 71, Notes: m.Notes, Required: false, File: m.Files[arm64]}
	if !reflect.DeepEqual(o, want) {
		t.Fatalf("offer %+v, want %+v", o, want)
	}
}

func TestCheckRefusesAManifestSignedByAnUnknownKey(t *testing.T) {
	pub, _ := newKey(t)
	_, other := newKey(t)
	o, err := check([]string{serve(t, answer(sealed(t, sample(), other)))}, []ed25519.PublicKey{pub}, 65, "install-1")
	if o != nil || !errors.Is(err, ErrSignature) {
		t.Fatalf("offer %+v, err = %v, want %v", o, err, ErrSignature)
	}
}

func TestCheckAcceptsTheSecondPinnedKey(t *testing.T) {
	old, _ := newKey(t)
	next, priv := newKey(t)
	o, err := check([]string{serve(t, answer(sealed(t, sample(), priv)))}, []ed25519.PublicKey{old, next}, 65, "install-1")
	if err != nil || o == nil || o.Build != 71 {
		t.Fatalf("offer %+v, err = %v", o, err)
	}
}

func TestCheckOffersNothingForTheSameOrAnOlderBuild(t *testing.T) {
	pub, priv := newKey(t)
	src := serve(t, answer(sealed(t, sample(), priv)))
	for _, current := range []int{71, 72, 1000} {
		o, err := check([]string{src}, []ed25519.PublicKey{pub}, current, "install-1")
		if o != nil || err != nil {
			t.Fatalf("current %d: offer %+v, err = %v", current, o, err)
		}
	}
}

func TestCheckOffersNothingWhenThisPlatformHasNoFile(t *testing.T) {
	pub, priv := newKey(t)
	m := sample()
	delete(m.Files, arm64)
	m.Min = 71
	o, err := check([]string{serve(t, answer(sealed(t, m, priv)))}, []ed25519.PublicKey{pub}, 1, "install-1")
	if o != nil || err != nil {
		t.Fatalf("offer %+v, err = %v", o, err)
	}
}

func TestCheckFollowsTheRolloutUnlessTheUpdateIsRequired(t *testing.T) {
	pub, priv := newKey(t)
	m := sample()
	m.Rollout = 25
	src := serve(t, answer(sealed(t, m, priv)))
	in, out := installIn(t, m.Build, true, m.Rollout), installIn(t, m.Build, false, m.Rollout)
	for _, c := range []struct {
		install  string
		current  int
		offered  bool
		required bool
	}{
		{in, 65, true, false},
		{out, 65, false, false},
		{out, m.Min, false, false},
		{out, m.Min - 1, true, true},
		{in, m.Min - 1, true, true},
	} {
		o, err := check([]string{src}, []ed25519.PublicKey{pub}, c.current, c.install)
		if err != nil {
			t.Fatal(err)
		}
		if (o != nil) != c.offered || o != nil && o.Required != c.required {
			t.Fatalf("%s at build %d: offer %+v, want offered %v required %v", c.install, c.current, o, c.offered, c.required)
		}
	}
}

func TestARequiredUpdateIgnoresARolloutOfZero(t *testing.T) {
	pub, priv := newKey(t)
	m := sample()
	m.Rollout = 0
	src := serve(t, answer(sealed(t, m, priv)))
	o, err := check([]string{src}, []ed25519.PublicKey{pub}, 59, "install-1")
	if err != nil || o == nil || !o.Required {
		t.Fatalf("offer %+v, err = %v", o, err)
	}
	o, err = check([]string{src}, []ed25519.PublicKey{pub}, 60, "install-1")
	if err != nil || o != nil {
		t.Fatalf("offer %+v, err = %v", o, err)
	}
}

func TestRolloutTakesItsShareOfInstallsWithinTwoPercent(t *testing.T) {
	const n = 10000
	for _, rollout := range []int{0, 1, 5, 25, 50, 75, 100} {
		for _, build := range []int{71, 72, 500} {
			got := 0
			for i := range n {
				if bucket("install-"+strconv.Itoa(i), build) < rollout {
					got++
				}
			}
			if want := n * rollout / 100; got < want-n/50 || got > want+n/50 {
				t.Errorf("rollout %d%% of build %d took %d of %d installs", rollout, build, got, n)
			}
		}
	}
}

func TestRolloutTakesOtherInstallsFirstForEachBuild(t *testing.T) {
	const n = 10000
	both := 0
	for i := range n {
		id := "install-" + strconv.Itoa(i)
		if bucket(id, 71) < 25 && bucket(id, 72) < 25 {
			both++
		}
	}
	if both < n/16-150 || both > n/16+150 {
		t.Fatalf("%d of %d installs are early for both builds, want about %d", both, n, n/16)
	}
}

func TestCheckMovesOnWhenTheFirstSourceFails(t *testing.T) {
	pub, priv := newKey(t)
	good := serve(t, answer(sealed(t, sample(), priv)))
	later := sample()
	later.Build = 80
	huge := append(sealed(t, later, priv), bytes.Repeat([]byte(" "), maxEnvelope)...)
	for name, first := range map[string]string{
		"connection refused": refused(t),
		"status 500":         serve(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusInternalServerError) }),
		"over 64 KB":         serve(t, answer(huge)),
		"garbage":            serve(t, answer([]byte("<html>Доступ ограничен</html>"))),
		"empty answer":       serve(t, answer(nil)),
	} {
		o, err := check([]string{first, good}, []ed25519.PublicKey{pub}, 65, "install-1")
		if err != nil || o == nil || o.Build != 71 {
			t.Fatalf("%s: offer %+v, err = %v", name, o, err)
		}
	}
}

func TestCheckMovesOnFromASourceThatNeverAnswers(t *testing.T) {
	defer func(d time.Duration) { sourceTimeout = d }(sourceTimeout)
	sourceTimeout = 200 * time.Millisecond
	stop := make(chan struct{})
	pub, priv := newKey(t)
	hang := serve(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-stop:
		}
	})
	t.Cleanup(func() { close(stop) })
	good := serve(t, answer(sealed(t, sample(), priv)))
	began := time.Now()
	o, err := check([]string{hang, good}, []ed25519.PublicKey{pub}, 65, "install-1")
	if err != nil || o == nil {
		t.Fatalf("offer %+v, err = %v", o, err)
	}
	if d := time.Since(began); d > 5*time.Second {
		t.Fatalf("took %v", d)
	}
}

func TestCheckTellsWhyNoSourceGaveAnUpdate(t *testing.T) {
	pub, priv := newKey(t)
	_, other := newKey(t)
	broken := sample()
	broken.Rollout = 101
	foreign := serve(t, answer(sealed(t, sample(), other)))
	invalid := serve(t, answer(signedRaw(t, encode(t, broken), priv)))
	down := refused(t)
	for _, c := range []struct {
		sources []string
		want    error
	}{
		{[]string{down, serve(t, answer([]byte("{}")))}, ErrUnreachable},
		{nil, ErrUnreachable},
		{[]string{down, invalid}, ErrManifest},
		{[]string{invalid, foreign, down}, ErrSignature},
		{[]string{foreign, invalid}, ErrSignature},
	} {
		o, err := check(c.sources, []ed25519.PublicKey{pub}, 65, "install-1")
		if o != nil || !errors.Is(err, c.want) {
			t.Fatalf("%v: offer %+v, err = %v, want %v", c.sources, o, err, c.want)
		}
		for _, other := range []error{ErrUnreachable, ErrSignature, ErrManifest} {
			if other != c.want && errors.Is(err, other) {
				t.Fatalf("%v: err = %v is also %v", c.sources, err, other)
			}
		}
	}
}

func TestCheckRefusesAManifestWithPlainHTTPLinks(t *testing.T) {
	pub, priv := newKey(t)
	m := sample()
	f := m.Files[arm64]
	f.URLs = []string{"http://a.example/u/Spofy-0.2.0-arm64.apk"}
	m.Files[arm64] = f
	o, err := check([]string{serve(t, answer(signedRaw(t, encode(t, m), priv)))}, []ed25519.PublicKey{pub}, 65, "install-1")
	if o != nil || !errors.Is(err, ErrManifest) {
		t.Fatalf("offer %+v, err = %v, want %v", o, err, ErrManifest)
	}
}

func TestCheckStopsAtOnceWhenTheCallerGivesUp(t *testing.T) {
	pub, priv := newKey(t)
	ctx, cancel := context.WithCancel(context.Background())
	var asked atomic.Int32
	first := serve(t, func(w http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
	})
	body := sealed(t, sample(), priv)
	second := serve(t, func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		w.Write(body)
	})
	o, err := Check(ctx, &http.Client{}, []string{first, second}, []ed25519.PublicKey{pub}, 65, arm64, "install-1")
	if o != nil || !errors.Is(err, context.Canceled) || asked.Load() != 0 {
		t.Fatalf("offer %+v, err = %v, second source asked %d times", o, err, asked.Load())
	}
}
