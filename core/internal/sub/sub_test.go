package sub

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"
)

func fixture(t *testing.T) ([]byte, http.Header) {
	t.Helper()
	body, err := os.ReadFile("../../testdata/sub/sub.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../testdata/sub/headers.json")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	h := http.Header{}
	for k, v := range m {
		h.Set(k, v)
	}
	return body, h
}

func TestParseFixture(t *testing.T) {
	body, h := fixture(t)
	p, err := Parse(body, h)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Servers) != 19 {
		t.Fatalf("servers = %d, want 19", len(p.Servers))
	}
	if p.Title != "🛡 Spofy VPN" {
		t.Fatalf("title = %q", p.Title)
	}
	if p.Announce != "Тестовое объявление" {
		t.Fatalf("announce = %q", p.Announce)
	}
	if p.UpdateEvery != time.Hour {
		t.Fatalf("update interval = %v", p.UpdateEvery)
	}
	if p.Usage.Download != 452716590 || p.Usage.Total != 107374182400 || p.Usage.Expire.Unix() != 1798761600 {
		t.Fatalf("usage = %+v", p.Usage)
	}
	if p.Flags["hwid-active"] != "true" {
		t.Fatalf("flags = %v", p.Flags)
	}
	if p.ETag == "" || p.SupportURL == "" || p.WebPageURL == "" {
		t.Fatalf("missing header fields: %+v", p)
	}
	ids := map[string]bool{}
	for _, s := range p.Servers {
		if s.ID == "" || s.Endpoint == "" || s.Label == "" || s.Protocol == "" {
			t.Fatalf("incomplete server: %+v", s.Name)
		}
		if ids[s.ID] {
			t.Fatalf("duplicate id %q", s.ID)
		}
		ids[s.ID] = true
	}
	first := p.Servers[0]
	if first.ID != "de" || first.Country != "DE" || first.Label != "(DE)" || first.Protocol != "vless" {
		t.Fatalf("first server = %+v", first)
	}
}

func TestIDIgnoresDecorations(t *testing.T) {
	plain := `[{"remarks":"🇩🇪 (DE)","outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"192.0.2.1","port":443}]}}]}]`
	fire := `[{"remarks":"🇩🇪 (DE) 🔥","outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"192.0.2.1","port":443}]}}]}]`
	a, err := Parse([]byte(plain), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Parse([]byte(fire), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if a.Servers[0].ID != b.Servers[0].ID || a.Servers[0].Endpoint != b.Servers[0].Endpoint {
		t.Fatalf("ids differ: %+v vs %+v", a.Servers[0], b.Servers[0])
	}
}

func TestDuplicateNamesGetDistinctIDs(t *testing.T) {
	body := `[{"remarks":"A","outbounds":[{"protocol":"vless"}]},{"remarks":"A","outbounds":[{"protocol":"trojan"}]}]`
	p, err := Parse([]byte(body), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Servers[0].ID != "a" || p.Servers[1].ID != "a-2" {
		t.Fatalf("ids = %q, %q", p.Servers[0].ID, p.Servers[1].ID)
	}
}

func TestParseRejectsOtherFormats(t *testing.T) {
	for _, body := range []string{"", "dmxlc3M6Ly8=", "vless://x@y:1", "[]", "[1,2]", `{"outbounds":[]}`, "{broken"} {
		if _, err := Parse([]byte(body), http.Header{}); !errors.Is(err, ErrFormat) {
			t.Fatalf("body %q: err = %v", body, err)
		}
	}
}

func TestFetch(t *testing.T) {
	body, h := fixture(t)
	var got http.Header
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		got = r.Header.Clone()
		if r.Header.Get("If-None-Match") == h.Get("Etag") {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		for k, v := range h {
			w.Header()[k] = v
		}
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		zw.Write(body)
		zw.Close()
	}))
	defer srv.Close()
	d := Device{HWID: "hw-1", OS: "Android", OSVersion: "15", Model: "Pixel 8", AppVersion: "0.1.0"}
	p, err := Fetch(context.Background(), srv.Client(), srv.URL, d, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Servers) != 19 {
		t.Fatalf("servers = %d", len(p.Servers))
	}
	if got.Get("User-Agent") != "Spofy/0.1.0 (Android 15; Pixel 8)" || got.Get("X-Hwid") != "hw-1" || got.Get("X-Device-Model") != "Pixel 8" {
		t.Fatalf("request headers = %v", got)
	}
	if _, err := Fetch(context.Background(), srv.Client(), srv.URL, d, p.ETag); !errors.Is(err, ErrNotModified) {
		t.Fatalf("second fetch err = %v", err)
	}
	if hits != 2 {
		t.Fatalf("hits = %d", hits)
	}
}

func TestFetchStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	_, err := Fetch(context.Background(), srv.Client(), srv.URL, Device{}, "")
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusBadGateway {
		t.Fatalf("err = %v", err)
	}
}

func notice(remarks, address string) string {
	b, _ := json.Marshal(remarks)
	return `{"remarks":` + string(b) + `,"outbounds":[{"tag":"proxy","protocol":"vless","settings":{"vnext":[{"address":"` + address + `","port":1,"users":[{"id":"00000000-0000-0000-0000-000000000000","encryption":"none"}]}]},"streamSettings":{"network":"tcp"}},{"tag":"direct","protocol":"freedom"}]}`
}

func TestEntriesWithoutAnAddressAreNoticesNotServers(t *testing.T) {
	p, err := Parse([]byte("["+notice("⏰ Подписка закончилась", "0.0.0.0")+","+notice(" VPN выключен до продления ", "::")+"]"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Servers) != 0 || !reflect.DeepEqual(p.Notes, []string{"⏰ Подписка закончилась", "VPN выключен до продления"}) {
		t.Fatalf("servers %d, notes %q", len(p.Servers), p.Notes)
	}
	body, h := fixture(t)
	mixed := append([]byte("["+notice("Обходы выключены", "0.0.0.0")+","), body[1:]...)
	whole, err := Parse(body, h)
	if err != nil {
		t.Fatal(err)
	}
	p, err = Parse(mixed, h)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Servers) != len(whole.Servers) || !reflect.DeepEqual(p.Notes, []string{"Обходы выключены"}) {
		t.Fatalf("servers %d of %d, notes %q", len(p.Servers), len(whole.Servers), p.Notes)
	}
	if _, err := Parse([]byte(`[{"remarks":"x","outbounds":[]}]`), nil); !errors.Is(err, ErrFormat) {
		t.Fatalf("a list with nothing usable: %v", err)
	}
}
