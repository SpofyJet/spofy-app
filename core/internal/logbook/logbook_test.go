package logbook

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func clock() func() time.Time {
	at := time.Unix(1795000000, 0)
	return func() time.Time { return at }
}

func ticking() (func() time.Time, func()) {
	at := time.Unix(1795000000, 0)
	return func() time.Time { return at }, func() { at = at.Add(time.Second) }
}

func TestRepeatsBecomeOneLineWithTheCountAndTheLastTime(t *testing.T) {
	now, tick := ticking()
	b := New(10, now)
	b.Add("app", "connected 🇩🇪 (DE)")
	for i := range 3 {
		tick()
		b.Add("engine", fmt.Sprintf("[Error] [%d] transport/internet: failed to resolve ip > features/dns: empty response", 1000+i))
		b.Add("engine", fmt.Sprintf("[Warning] [%d] proxy/vless/outbound: connection ends > io: read/write on closed pipe", 2000+i))
	}
	b.Add("app", "connecting 🇮🇪 (IE)")
	b.Add("app", "connecting 🇮🇪 (IE)")
	b.Add("app", "connected 🇮🇪 (IE)")
	tick()
	b.Add("engine", "[Error] [7] transport/internet: failed to resolve ip > features/dns: empty response")
	b.Add("app", "connected 🇩🇪 (DE)")
	got := b.Lines()
	want := []Line{
		{At: 1795000000, Source: "app", Text: "connected 🇩🇪 (DE)"},
		{At: 1795000001, Last: 1795000003, Count: 3, Source: "engine", Text: "[Warning] [2002] proxy/vless/outbound: connection ends > io: read/write on closed pipe"},
		{At: 1795000003, Last: 1795000003, Count: 2, Source: "app", Text: "connecting 🇮🇪 (IE)"},
		{At: 1795000003, Source: "app", Text: "connected 🇮🇪 (IE)"},
		{At: 1795000001, Last: 1795000004, Count: 4, Source: "engine", Cause: "direct_name", Text: "[Error] [7] transport/internet: failed to resolve ip > features/dns: empty response"},
		{At: 1795000004, Source: "app", Text: "connected 🇩🇪 (DE)"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("lines\n%+v\nwant\n%+v", got, want)
	}
}

func TestKnownCausesAreNamedAndTheCoresNoiseWaitsForTheDetailedLog(t *testing.T) {
	lines := map[string]string{
		"[Error] [3] transport/internet: failed to resolve ip > features/dns: empty response":                                       "direct_name",
		"[Error] app/dns: failed to retrieve response for mtalk.example.test. > Post \"https://8.8.8.8/dns-query\": unexpected EOF": "name_server",
		"[Info] app/dns/fakedns: A fake ip request to 198.19.127.1, however there is no matching domain name in fake DNS":           "fake_address",
		"[Info] app/router: fallback to [fb], due to empty tag returned":                                                            "reserve",
		"[Warning] app/observatory/burst: error ping http://check.example.test/generate_204 with proxy: context deadline exceeded":  "server_check",
		"[Warning] app/observatory/burst: network is down":                                                                          "no_network",
		"[Warning] [9] app/dispatcher: proxy/vless/outbound: failed to find an available destination > dial tcp: i/o timeout":       "server_silent",
		"[Error] [9] app/proxyman/inbound: connection ends > something new":                                                         "",
	}
	noise := []string{
		`[Warning] infra/conf: The "freedom.domainStrategy" setting is deprecated and will be removed.`,
		`[Warning] common/errors: The feature gRPC transport (with unnecessary costs, etc.) is deprecated, not recommended for using and might be removed.`,
		`[Warning] infra/conf: Defined FakeDNS but haven't enabled FakeDNS destOverride at any inbound.`,
		`[Warning] core: Xray 26.11.9 started`,
	}
	b := New(100, clock())
	for text := range lines {
		b.Add("engine", text)
	}
	for _, text := range noise {
		b.Add("engine", text)
	}
	got := b.Lines()
	if len(got) != len(lines) {
		t.Fatalf("%d lines by default: %+v", len(got), got)
	}
	for _, l := range got {
		if l.Cause != lines[l.Text] {
			t.Errorf("%q: cause %q, want %q", l.Text, l.Cause, lines[l.Text])
		}
	}
	b.Verbose(true)
	for _, text := range noise {
		b.Add("engine", text)
	}
	if n := len(b.Lines()); n != len(lines)+len(noise) {
		t.Fatalf("%d lines in the detailed log", n)
	}
}

func TestARepeatedLineStaysMasked(t *testing.T) {
	b := New(10, clock())
	b.Hide([]string{"node.example.test"})
	b.Add("engine", "[Error] [1] dial tcp node.example.test:443: i/o timeout")
	b.Add("engine", "[Error] [2] dial tcp node.example.test:443: i/o timeout")
	if got := b.Lines(); len(got) != 1 || got[0].Count != 2 || got[0].Text != "[Error] [2] dial tcp •••:443: i/o timeout" {
		t.Fatalf("%+v", got)
	}
}

func TestTheBookStaysSmallUnderASpammingCore(t *testing.T) {
	b := New(400, clock())
	for i := range 10000 {
		b.Add("engine", fmt.Sprintf("[Error] [%d] transport/internet: failed to resolve ip > features/dns: empty response", i))
		b.Add("engine", fmt.Sprintf("[Info] [%d] app/dispatcher: taking detour [proxy] for [tcp:site%d.example.test:443]", i, i%50))
	}
	got := b.Lines()
	data, _ := json.Marshal(got)
	if len(got) != 51 || got[len(got)-1].Count != 200 || len(data) > 16<<10 {
		t.Fatalf("%d lines, %d bytes", len(got), len(data))
	}
}

func BenchmarkAdd(b *testing.B) {
	book := New(400, clock())
	book.Hide([]string{"node.example.test", "sub.example.test/TOKEN"})
	for i := range b.N {
		book.Add("engine", fmt.Sprintf("[Info] [%d] app/dispatcher: taking detour [proxy] for [tcp:site%d.example.test:443]", i, i%1000))
	}
}

func TestTheBookKeepsTheLastLinesInOrder(t *testing.T) {
	b := New(3, clock())
	for i := range 5 {
		b.Add("engine", fmt.Sprint(i))
	}
	got := b.Lines()
	if len(got) != 3 || got[0].Text != "2" || got[2].Text != "4" || got[0].At != 1795000000 || got[0].Source != "engine" {
		t.Fatalf("lines = %+v", got)
	}
	b.Clear()
	b.Add("app", "after")
	if got := b.Lines(); len(got) != 1 || got[0].Text != "after" {
		t.Fatalf("after clearing: %+v", got)
	}
}

func TestSecretsNeverReachTheBook(t *testing.T) {
	b := New(10, clock())
	b.Hide([]string{"node.example.test", "https://sub.example.test/sub/TOKEN123", "sub.example.test", "ab", ""})
	b.Add("engine", "dial tcp node.example.test:443 for 5f0c1f84-2b0a-4c9d-8e71-0123456789ab via https://sub.example.test/sub/TOKEN123 ab")
	b.Add("engine", "line\x00with\ncontrols")
	got := b.Lines()
	if strings.Contains(got[0].Text, "example") || strings.Contains(got[0].Text, "TOKEN") || strings.Contains(got[0].Text, "5f0c1f84") {
		t.Fatalf("leaked: %q", got[0].Text)
	}
	if !strings.HasSuffix(got[0].Text, " ab") || got[1].Text != "line with controls" {
		t.Fatalf("over-hidden or kept controls: %+v", got)
	}
}

func TestLongLinesAreCutOnARuneBoundary(t *testing.T) {
	b := New(1, clock())
	b.Add("engine", strings.Repeat("ж", 400))
	got := b.Lines()[0].Text
	if len(got) > maxText+len("…") || !strings.HasSuffix(got, "…") || strings.ContainsRune(got, '�') {
		t.Fatalf("len %d", len(got))
	}
}

func TestTheBookTakesLinesFromManyWriters(t *testing.T) {
	b := New(100, clock())
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for j := range 50 {
				b.Add("engine", fmt.Sprint(i, j))
				if j%10 == 0 {
					b.Hide([]string{fmt.Sprint("host", i)})
					b.Lines()
				}
			}
		})
	}
	wg.Wait()
	if n := len(b.Lines()); n != 100 {
		t.Fatalf("kept %d", n)
	}
}
