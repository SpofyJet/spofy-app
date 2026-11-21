package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fetchTarget(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sub":
			w.Header().Set("Subscription-Userinfo", "upload=1; download=2")
			w.Write([]byte(r.Header.Get("User-Agent") + "|" + r.Header.Get("X-Hwid")))
		case "/moved":
			http.Redirect(w, r, "/sub", http.StatusMovedPermanently)
		case "/huge":
			w.Write(bytes.Repeat([]byte("x"), FetchMax+1))
		case "/full":
			w.Write(bytes.Repeat([]byte{0xff}, FetchMax))
		}
	}))
	t.Cleanup(srv.Close)
	roots = srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	t.Cleanup(func() { roots = nil })
	return srv
}

func TestFetchCarriesOneRoundTripThroughTheChosenOutbound(t *testing.T) {
	srv := fetchTarget(t)
	inst, err := Start([]byte(probeConfig))
	if err != nil {
		t.Fatal(err)
	}
	defer inst.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	f, err := inst.Fetch(ctx, srv.URL+"/sub", "proxy", map[string]string{"User-Agent": "Spofy/1", "X-Hwid": "hw"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != http.StatusOK || string(f.Body) != "Spofy/1|hw" || http.Header(f.Header).Get("Subscription-Userinfo") == "" {
		t.Fatalf("fetched = %d %q %v", f.Status, f.Body, f.Header)
	}
	f, err = inst.Fetch(ctx, srv.URL+"/moved", "proxy", nil)
	if err != nil || f.Status != http.StatusMovedPermanently || !strings.HasSuffix(http.Header(f.Header).Get("Location"), "/sub") {
		t.Fatalf("a redirect must come back to the caller untouched: %+v, err = %v", f, err)
	}
	for _, c := range []struct {
		url, tag string
		code     Code
	}{
		{srv.URL + "/sub", "dead", CodeFetch},
		{srv.URL + "/huge", "proxy", CodeFetch},
		{strings.Replace(srv.URL, "https://", "http://", 1) + "/sub", "proxy", CodeConfig},
		{"://", "proxy", CodeConfig},
	} {
		if _, err := inst.Fetch(ctx, c.url, c.tag, nil); probeCode(err) != c.code {
			t.Fatalf("%s via %q: err = %v", c.url, c.tag, err)
		}
	}
	roots = nil
	if _, err := inst.Fetch(ctx, srv.URL+"/sub", "proxy", nil); probeCode(err) != CodeFetch {
		t.Fatalf("a certificate nobody vouches for was accepted: err = %v", err)
	}
}

func TestTheLargestAnswerFitsOneLineOfTheProtocol(t *testing.T) {
	srv := fetchTarget(t)
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	go Serve(inR, outW)
	defer inW.Close()
	replies := make(chan Reply, 16)
	go func() {
		lines := bufio.NewScanner(outR)
		lines.Buffer(make([]byte, 64<<10), maxLine)
		for lines.Scan() {
			var m Reply
			if json.Unmarshal(lines.Bytes(), &m) == nil && m.Re != 0 {
				replies <- m
			}
		}
		close(replies)
	}()
	for _, req := range []Request{
		{ID: 1, Op: "start", Config: json.RawMessage(probeConfig)},
		{ID: 2, Op: "fetch", URL: srv.URL + "/full", Tag: "proxy", Timeout: 10000},
	} {
		b, _ := json.Marshal(req)
		go inW.Write(append(b, '\n'))
		select {
		case m, ok := <-replies:
			if !ok {
				t.Fatalf("%s: the stream broke on a line the protocol must carry", req.Op)
			}
			if m.Re != req.ID || !m.OK || (req.Op == "fetch" && (m.Fetched == nil || len(m.Fetched.Body) != FetchMax)) {
				t.Fatalf("%s: %+v", req.Op, m.Code)
			}
		case <-time.After(15 * time.Second):
			t.Fatalf("%s: no reply", req.Op)
		}
	}
}
