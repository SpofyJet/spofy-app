package tray

import (
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
