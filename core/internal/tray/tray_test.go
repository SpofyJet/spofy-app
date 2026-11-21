package tray

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode/utf16"
)

const samples = "../../../proto/samples/session.jsonl"
const dart = "../../../app/lib/ui/strings.dart"

func block(t *testing.T, src, head string) string {
	t.Helper()
	i := strings.Index(src, head)
	if i < 0 {
		t.Fatalf("strings.dart has no %q", head)
	}
	rest := src[i:]
	return rest[:strings.Index(rest, "\n  };")]
}

var entry = regexp.MustCompile(`(?m)^\s+((?:'\w+'(?: \|\| )?)+|VpnState\.\w+|_) =>\s+(\w+|'[^'$]*'),?$`)

func pairs(text string) map[string]string {
	out := map[string]string{}
	for _, m := range entry.FindAllStringSubmatch(text, -1) {
		for _, key := range regexp.MustCompile(`\w+`).FindAllString(m[1], -1) {
			if key != "VpnState" {
				out[key] = m[2]
			}
		}
	}
	return out
}

func TestTextMatchesTheWindow(t *testing.T) {
	raw, err := os.ReadFile(dart)
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	consts := map[string]string{}
	for _, m := range regexp.MustCompile(`static const (\w+) =\s+'([^']*)';`).FindAllStringSubmatch(src, -1) {
		consts[m[1]] = m[2]
	}
	text := func(v string) string {
		if strings.HasPrefix(v, "'") {
			return strings.Trim(v, "'")
		}
		c, ok := consts[v]
		if !ok {
			t.Fatalf("strings.dart has no constant %s", v)
		}
		return c
	}
	same := func(what, got, want string) {
		t.Helper()
		if got != want {
			t.Errorf("%s: the tray says %q, the window says %q", what, got, want)
		}
	}
	for name, got := range map[string]string{
		"app": textApp, "connect": textConnect, "disconnect": textDisconnect, "stop": textStop,
		"addSubscription": textAddSubscription, "renew": textRenew, "manage": textManage, "coreProtocol": textStale,
	} {
		same(name, got, consts[name])
	}
	window := pairs(block(t, src, "static String title(VpnState state)"))
	if len(window) != len(titles) {
		t.Errorf("the window names %d states, the tray %d", len(window), len(titles))
	}
	for state, v := range window {
		same("title "+state, Title(state), text(v))
	}
	same("hint stalled", Hint("stalled"), text(pairs(block(t, src, "static String hint("))["stalled"]))
	reasonBlock := block(t, src, "static String reason(String code)")
	if !strings.Contains(reasonBlock, "_ => 'Ошибка: $code'") || !strings.Contains(reasonBlock, "'' => ''") {
		t.Error("the window's fallback reasons changed")
	}
	window = pairs(reasonBlock)
	if read, cases := len(entry.FindAllString(reasonBlock, -1)), strings.Count(reasonBlock, "=>")-3; read != cases {
		t.Errorf("the check reads %d of the window's %d reasons", read, cases)
	}
	if len(window) != len(reasons) {
		t.Errorf("the window explains %d codes, the tray %d", len(window), len(reasons))
	}
	for code, v := range window {
		same("reason "+code, Reason(code), text(v))
	}
	same("reason of an unknown code", Reason("teleport"), "Ошибка: teleport")
	same("reason of no code", Reason(""), "")
	for _, f := range []struct {
		head string
		fn   func(string) string
	}{{"static String stopTitle(", StopTitle}, {"static String stopAction(", StopAction}} {
		window = pairs(block(t, src, f.head))
		if len(window) != 4 {
			t.Errorf("%s: %d branches in the window", f.head, len(window))
		}
		for stop, v := range window {
			key := stop
			if stop == "_" {
				key = "paused"
			}
			same(f.head+stop, f.fn(key), text(v))
		}
	}
}

func feed(t *testing.T, m *Model, lines ...string) []Result {
	t.Helper()
	var out []Result
	for _, line := range lines {
		r, err := m.Read([]byte(line))
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		out = append(out, r)
	}
	return out
}

func conn(state, server, code string) string {
	b, _ := json.Marshal(map[string]any{"ev": "connection", "connection": map[string]any{"state": state, "choice": "auto", "server": server, "delayMs": 0, "code": code, "error": ""}})
	return string(b)
}

const hello = `{"re":1,"ok":true,"snapshot":{"version":"0.1.0","protocol":1,"connection":{"state":"idle","choice":"auto","server":"","delayMs":0,"code":"","error":""},"subscription":{"title":"t","upload":0,"download":0,"total":0,"expire":0,"supportUrl":"","webPageUrl":"","announce":"","fetchedAt":0,"updateEvery":3600,"stop":"","notes":[]},"servers":[{"id":"nl","name":"🇳🇱 (NL)","label":"(NL)","country":"NL","protocol":"vless","tier":0,"delayMs":0,"measuredAt":0},{"id":"x","name":"x","label":"  ","country":"","protocol":"vless","tier":0,"delayMs":0,"measuredAt":0}],"settings":{}}}`

func linked(t *testing.T) *Model {
	m := &Model{}
	m.Link(1)
	feed(t, m, hello)
	if !m.Linked() {
		t.Fatal("the hello reply did not link the tray")
	}
	return m
}

func TestTheRecordedSessionDrivesTheTray(t *testing.T) {
	data, err := os.ReadFile(samples)
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{}
	m.Link(1)
	var notes []Note
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if body, ok := strings.CutPrefix(line, "> "); ok {
			var req struct {
				ID uint64 `json:"id"`
				Op string `json:"op"`
			}
			json.Unmarshal([]byte(body), &req)
			switch req.Op {
			case "connect":
				m.Ask(req.ID, Connect)
			case "disconnect":
				m.Ask(req.ID, Disconnect)
			}
			continue
		}
		r := feed(t, m, strings.TrimPrefix(line, "< "))[0]
		if r.Note != nil {
			notes = append(notes, *r.Note)
		}
		v := m.View()
		if v.Status == "" || v.Tip == "" || v.Toggle.Label == "" || v.Quit.Label == "" {
			t.Fatalf("after %s the menu has an empty line: %+v", line, v)
		}
		if strings.HasPrefix(line, `< {"ev":"connection","connection":{"state":"connected"`) && v.Icon != On {
			t.Fatalf("after %s the icon is %d", line, v.Icon)
		}
	}
	want := []string{"Сначала добавьте подписку", "Этот сервер не отвечает. Выберите другой или «Авто»", "Такого сервера больше нет в подписке"}
	if len(notes) != len(want) {
		t.Fatalf("notes = %+v, want %v", notes, want)
	}
	for i, n := range notes {
		if n.Title != "Нет подключения" || n.Text != want[i] {
			t.Errorf("note %d = %+v, want %q", i, n, want[i])
		}
	}
	if v := m.View(); v.Toggle.Action != Open || v.Toggle.Label != "Добавить подписку" {
		t.Errorf("after unsubscribing the toggle is %+v", v.Toggle)
	}
}

func TestEachStateHasItsIconAndToggle(t *testing.T) {
	for _, c := range []struct {
		state  string
		icon   Icon
		toggle Item
		quit   Action
	}{
		{"idle", Off, Item{Label: "Включить VPN", Action: Connect}, Nothing},
		{"failed", Off, Item{Label: "Включить VPN", Action: Connect}, Nothing},
		{"measuring", Busy, Item{Label: "Остановить подключение", Action: Disconnect}, Disconnect},
		{"connecting", Busy, Item{Label: "Остановить подключение", Action: Disconnect}, Disconnect},
		{"switching", Busy, Item{Label: "Остановить подключение", Action: Disconnect}, Disconnect},
		{"connected", On, Item{Label: "Выключить VPN", Action: Disconnect}, Disconnect},
		{"stalled", Alert, Item{Label: "Выключить VPN", Action: Disconnect}, Disconnect},
		{"offline", Alert, Item{Label: "Выключить VPN", Action: Disconnect}, Disconnect},
	} {
		m := linked(t)
		feed(t, m, conn(c.state, "nl", ""))
		v := m.View()
		if v.Icon != c.icon || v.Toggle != c.toggle || v.Quit.Action != c.quit || v.Stalled != (c.state == "stalled") {
			t.Errorf("%s: %+v", c.state, v)
		}
	}
}

func TestTheStatusNamesTheServer(t *testing.T) {
	m := linked(t)
	feed(t, m, conn("connected", "nl", ""))
	if v := m.View(); v.Status != "VPN работает · (NL)" || v.Tip != "Spofy\nVPN работает · (NL)" {
		t.Errorf("view = %+v", v)
	}
	feed(t, m, conn("connected", "x", ""))
	if v := m.View(); v.Status != "VPN работает · x" {
		t.Errorf("a blank label must fall back to the id: %q", v.Status)
	}
	feed(t, m, conn("connected", "gone", ""))
	if v := m.View(); v.Status != "VPN работает · gone" {
		t.Errorf("an unknown server must show its id: %q", v.Status)
	}
	feed(t, m, `{"ev":"servers","servers":[{"id":"gone","name":"g","label":"(GN)","country":"","protocol":"vless","tier":0,"delayMs":0,"measuredAt":0}]}`)
	if v := m.View(); v.Status != "VPN работает · (GN)" {
		t.Errorf("new servers were not taken: %q", v.Status)
	}
}

func TestOnlyALiveFailureIsAnnouncedOnce(t *testing.T) {
	m := &Model{}
	m.Link(1)
	failed := strings.Replace(hello, `"state":"idle","choice":"auto","server":"","delayMs":0,"code":""`, `"state":"failed","choice":"auto","server":"","delayMs":0,"code":"no_server"`, 1)
	if r := feed(t, m, failed)[0]; r.Note != nil {
		t.Errorf("a failure that happened before the tray started was announced: %+v", r.Note)
	}
	m = linked(t)
	r := feed(t, m, conn("connected", "nl", ""), conn("stalled", "", "server_unreachable"), conn("failed", "", "no_server"), conn("failed", "", "no_server"))
	if r[0].Note != nil || r[1].Note != nil || r[3].Note != nil {
		t.Errorf("notes outside the failure: %+v", r)
	}
	if r[2].Note == nil || *r[2].Note != (Note{Title: "Нет подключения", Text: "Ни один сервер не ответил. Проверьте интернет"}) {
		t.Errorf("the failure note = %+v", r[2].Note)
	}
	m = linked(t)
	m.Ask(7, Connect)
	r = feed(t, m, conn("connecting", "nl", ""), conn("failed", "", "no_server"), `{"re":7,"ok":false,"code":"no_server","error":"x"}`)
	if r[1].Note == nil || r[2].Note != nil || r[2].Answered != 7 {
		t.Errorf("a failed connect from the tray was announced %+v then %+v", r[1].Note, r[2])
	}
	m = linked(t)
	m.Ask(8, Connect)
	r = feed(t, m, `{"re":8,"ok":false,"code":"sub_ended","error":"x"}`)
	if r[0].Note == nil || r[0].Note.Text != "Подписка закончилась" {
		t.Errorf("a refused connect was not explained: %+v", r[0])
	}
	m.Ask(9, Disconnect)
	if r = feed(t, m, `{"re":9,"ok":true}`, `{"re":9,"ok":true}`, `{"re":5,"ok":false,"code":"no_server"}`); r[0].Answered != 9 || r[1].Answered != 0 || r[2].Note != nil {
		t.Errorf("replies to other requests leaked: %+v", r)
	}
}

func TestStalledNoteOnlyWhileStalled(t *testing.T) {
	m := linked(t)
	if m.StalledNote() != nil {
		t.Error("a note while idle")
	}
	feed(t, m, conn("stalled", "", "server_unreachable"))
	if n := m.StalledNote(); n == nil || *n != (Note{Title: "Сервер не отвечает", Text: "Трафик не идёт мимо VPN. Пробуем снова"}) {
		t.Errorf("note = %+v", n)
	}
	if m.StalledNote() != nil {
		t.Error("the same stall was announced twice")
	}
	feed(t, m, conn("connected", "nl", ""))
	if m.StalledNote() != nil {
		t.Error("a stalled note after the server came back")
	}
	m.Ask(5, Connect)
	r := feed(t, m, conn("stalled", "", "server_unreachable"), `{"re":5,"ok":false,"code":"server_unreachable"}`)
	if r[1].Note == nil || r[1].Note.Text != "Этот сервер не отвечает. Выберите другой или «Авто»" || m.StalledNote() != nil {
		t.Errorf("a refused connect into a stall: %+v, then a second note", r[1].Note)
	}
	feed(t, m, conn("connecting", "nl", ""), conn("stalled", "", "server_unreachable"))
	if m.StalledNote() == nil {
		t.Error("a new stall was not announced")
	}
	m.Lost()
	if m.StalledNote() != nil {
		t.Error("a stalled note without the service")
	}
}

func TestWithoutTheServiceTheMenuWaits(t *testing.T) {
	m := &Model{}
	v := m.View()
	if v.Status != "Служба Spofy не отвечает" || !v.Toggle.Grey || v.Toggle.Action != Nothing || v.Quit.Action != Nothing || v.Icon != Off {
		t.Errorf("view = %+v", v)
	}
	m.Link(3)
	feed(t, m, conn("connected", "nl", ""), `{"re":3,"ok":false,"code":"bad_request"}`)
	if m.Linked() || m.View().Icon != Off {
		t.Error("events before the hello reply or a refused hello linked the tray")
	}
	m.Link(4)
	feed(t, m, strings.Replace(strings.Replace(hello, `"re":1`, `"re":4`, 1), `"protocol":1`, `"protocol":2`, 1))
	if v := m.View(); m.Linked() || v.Status != "Части приложения разных версий. Обновите Spofy" || !v.Toggle.Grey {
		t.Errorf("another protocol: %+v", v)
	}
	if _, err := m.Read([]byte("{")); err == nil {
		t.Error("a broken line was accepted")
	}
	m = linked(t)
	feed(t, m, conn("connected", "nl", ""))
	m.Lost()
	if v := m.View(); m.Linked() || v.Icon != Off || v.Status != "Служба Spofy не отвечает" {
		t.Errorf("after losing the service: %+v", v)
	}
}

func TestStoppedSubscriptionPointsToTheWindow(t *testing.T) {
	m := linked(t)
	feed(t, m, `{"ev":"subscription","subscription":{"title":"t","upload":0,"download":0,"total":0,"expire":0,"supportUrl":"","webPageUrl":"","announce":"","fetchedAt":0,"updateEvery":3600,"stop":"spent","notes":[]}}`)
	if v := m.View(); v.Status != "Трафик закончился" || v.Toggle != (Item{Label: "Докупить трафик", Action: Open}) {
		t.Errorf("view = %+v", v)
	}
	feed(t, m, conn("connected", "nl", ""))
	if v := m.View(); v.Status != "VPN работает · (NL)" || v.Toggle.Action != Disconnect {
		t.Errorf("a running VPN must stay controllable: %+v", v)
	}
	feed(t, m, conn("idle", "", ""), `{"ev":"subscription","subscription":null}`)
	if v := m.View(); v.Toggle != (Item{Label: "Добавить подписку", Action: Open}) {
		t.Errorf("without a subscription: %+v", v)
	}
}

func TestTheTipFitsTheShell(t *testing.T) {
	long := strings.Repeat("🔥", 80)
	got := Clip(long, 127)
	if n := len(utf16.Encode([]rune(got))); n > 127 || n < 126 || !strings.HasPrefix(long, got) {
		t.Errorf("clipped to %d units", n)
	}
	if Clip("VPN", 127) != "VPN" {
		t.Error("a short tip was clipped")
	}
	m := linked(t)
	feed(t, m, `{"ev":"servers","servers":[{"id":"nl","name":"n","label":"`+long+`","country":"","protocol":"vless","tier":0,"delayMs":0,"measuredAt":0}]}`, conn("connected", "nl", ""))
	if n := len(utf16.Encode([]rune(m.View().Tip))); n > 127 {
		t.Errorf("the tip has %d units", n)
	}
}
