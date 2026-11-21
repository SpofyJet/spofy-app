package tray

import (
	"encoding/xml"
	"fmt"
	"github.com/SpofyJet/spofy-app/core/internal/app"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestTheReminderComesFromNoonOfTheChosenDayAndOnceTheEndIsReached(t *testing.T) {
	zone := time.FixedZone("MSK", 3*3600)
	end := time.Date(2027, 1, 10, 9, 30, 0, 0, zone)
	expire := end.Unix()
	at := func(month time.Month, day, hour, minute int) time.Time {
		return time.Date(2027, month, day, hour, minute, 0, 0, zone)
	}
	for _, c := range []struct {
		name  string
		now   time.Time
		days  int
		due   bool
		ended bool
		left  int
	}{
		{"a minute before noon three days ahead", at(1, 7, 11, 59), 3, false, false, 0},
		{"noon three days ahead", at(1, 7, 12, 0), 3, true, false, 3},
		{"the phone was off at noon", at(1, 8, 20, 0), 3, true, false, 2},
		{"the evening before", at(1, 9, 23, 0), 3, true, false, 1},
		{"the last morning", at(1, 10, 8, 0), 3, true, false, 0},
		{"one day ahead is chosen", at(1, 8, 13, 0), 1, false, false, 0},
		{"one day ahead at its noon", at(1, 9, 12, 0), 1, true, false, 1},
		{"seven days ahead", at(1, 3, 12, 0), 7, true, false, 7},
		{"the end", at(1, 10, 9, 30), 3, true, true, 0},
		{"two days after the end", at(1, 12, 9, 0), 3, true, true, 0},
		{"long after the end", at(1, 14, 9, 30), 3, false, false, 0},
		{"switched off", at(1, 9, 12, 0), 0, false, false, 0},
	} {
		r, due := Due(expire, c.days, c.now)
		if due != c.due || r.Ended != c.ended || r.Left != c.left {
			t.Fatalf("%s: due=%v %+v", c.name, due, r)
		}
	}
	soon, _ := Due(expire, 3, at(1, 8, 12, 0))
	later, _ := Due(expire, 3, at(1, 9, 12, 0))
	ended, _ := Due(expire, 3, at(1, 11, 12, 0))
	renewed, _ := Due(expire+30*86400, 3, at(2, 8, 12, 0))
	if soon.Key != later.Key || soon.Key == ended.Key || soon.Key == renewed.Key {
		t.Fatalf("one reminder per end and kind: %q %q %q %q", soon.Key, later.Key, ended.Key, renewed.Key)
	}
	for _, expire := range []int64{0, -1, 4070908800, 4102444800} {
		if _, due := Due(expire, 3, at(1, 9, 12, 0)); due {
			t.Fatalf("expire %d: no end, no reminder", expire)
		}
	}
}

func TestTheTrayRemindsOnceInTheChosenLanguageAndKeepsQuietWhenOff(t *testing.T) {
	zone := time.FixedZone("MSK", 3*3600)
	end := time.Date(2027, 1, 10, 9, 30, 0, 0, zone)
	now := time.Date(2027, 1, 8, 12, 0, 0, 0, zone)
	m := linked(t)
	if key, n := m.Reminder(now); n != nil {
		t.Fatalf("a subscription without an end reminded: %s %+v", key, n)
	}
	sub := `{"ev":"subscription","subscription":{"title":"t","upload":0,"download":0,"total":0,"expire":` + strconv.FormatInt(end.Unix(), 10) + `,"supportUrl":"","webPageUrl":"","announce":"","fetchedAt":0,"updateEvery":3600,"stop":"","notes":[]}}`
	feed(t, m, sub)
	key, n := m.Reminder(now)
	if n == nil || *n != (Note{Title: "Подписка закончится через 2 дня", Text: "Продлите её, чтобы VPN работал без перерыва"}) {
		t.Fatalf("reminder: %q %+v", key, n)
	}
	m.Reminded(key)
	if _, n := m.Reminder(now.Add(time.Hour)); n != nil {
		t.Fatalf("the same reminder came twice: %+v", n)
	}
	feed(t, m, `{"ev":"settings","settings":{"reminded":"`+key+`","language":"en"}}`)
	if _, n := m.Reminder(end.Add(time.Hour)); n == nil || n.Title != "Your subscription has ended" {
		t.Fatalf("the end in English: %+v", n)
	}
	feed(t, m, `{"ev":"settings","settings":{"remind":0}}`)
	if _, n := m.Reminder(end.Add(time.Hour)); n != nil {
		t.Fatalf("reminders are off but one came: %+v", n)
	}
	m.Lost()
	if _, n := m.Reminder(now); n != nil {
		t.Fatal("a tray without the service reminded")
	}
}

func TestTheTrayRemindsInTheWordsOfTheAndroidApp(t *testing.T) {
	for l, folder := range map[Lang]string{Ru: "values", En: "values-en"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "..", "app", "android", "app", "src", "main", "res", folder, "strings.xml"))
		if err != nil {
			t.Fatal(err)
		}
		var res struct {
			Strings []struct {
				Name string `xml:"name,attr"`
				Text string `xml:",chardata"`
			} `xml:"string"`
			Plurals []struct {
				Name  string `xml:"name,attr"`
				Items []struct {
					Quantity string `xml:"quantity,attr"`
					Text     string `xml:",chardata"`
				} `xml:"item"`
			} `xml:"plurals"`
		}
		if err := xml.Unmarshal(b, &res); err != nil {
			t.Fatal(err)
		}
		android := map[string]string{}
		for _, s := range res.Strings {
			android[s.Name] = s.Text
		}
		for _, p := range res.Plurals {
			for _, item := range p.Items {
				android[p.Name+"."+item.Quantity] = item.Text
			}
		}
		same := func(what, tray, app string) {
			t.Helper()
			if tray != app {
				t.Errorf("%d %s: the tray says %q, Android %q", l, what, tray, app)
			}
		}
		same("ended", l.Remind(Reminder{Ended: true}).Title, android["remind_ended"])
		same("ended hint", l.Remind(Reminder{Ended: true}).Text, android["remind_ended_hint"])
		same("today", l.Remind(Reminder{}).Title, android["remind_today"])
		same("tomorrow", l.Remind(Reminder{Left: 1}).Title, android["remind_tomorrow"])
		same("soon hint", l.Remind(Reminder{Left: 4}).Text, android["remind_soon_hint"])
		quantity := map[Lang]map[int]string{Ru: {21: "one", 2: "few", 23: "few", 5: "many", 11: "many", 12: "many", 25: "many"}, En: {21: "other", 2: "other", 5: "other"}}
		for n, q := range quantity[l] {
			same(fmt.Sprintf("in %d days", n), l.Remind(Reminder{Left: n}).Title, fmt.Sprintf(android["remind_soon."+q], n))
		}
		if l == En {
			same("one day form", fmt.Sprintf(texts[En].remindSoon[0], 1), fmt.Sprintf(android["remind_soon.one"], 1))
		}
	}
}

func TestTheTrayKeepsWhatWasShownUnderTheCoreSettingKey(t *testing.T) {
	if RemindedKey != app.KeyReminded {
		t.Fatalf("tray %q, core %q", RemindedKey, app.KeyReminded)
	}
}
