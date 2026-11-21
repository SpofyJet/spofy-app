package tray

import (
	"strconv"
	"time"
)

const (
	RemindedKey   = "reminded"
	RemindDefault = 3
	lifetime      = 4070908800
	late          = 3 * 24 * time.Hour
)

type Reminder struct {
	Key   string
	Ended bool
	Left  int
}

func Due(expire int64, days int, now time.Time) (Reminder, bool) {
	if expire <= 0 || expire >= lifetime || days <= 0 {
		return Reminder{}, false
	}
	end := time.Unix(expire, 0).In(now.Location())
	key := strconv.FormatInt(expire, 10)
	if !now.Before(end) {
		if now.Sub(end) < late {
			return Reminder{Key: "1:" + key, Ended: true}, true
		}
		return Reminder{}, false
	}
	if now.Before(noon(end.AddDate(0, 0, -days))) {
		return Reminder{}, false
	}
	return Reminder{Key: "0:" + key, Left: between(now, end)}, true
}

func noon(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, t.Location())
}

func between(from, to time.Time) int {
	a := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	b := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(b.Sub(a).Hours() / 24)
}
