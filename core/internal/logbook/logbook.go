package logbook

import (
	"cmp"
	"container/list"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

type Line struct {
	At     int64  `json:"at"`
	Last   int64  `json:"last,omitempty"`
	Count  int    `json:"count,omitempty"`
	Source string `json:"source"`
	Cause  string `json:"cause,omitempty"`
	Text   string `json:"text"`
}

const (
	Hidden  = "•••"
	maxText = 512
	minWord = 4
)

var (
	uuid    = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	session = regexp.MustCompile(`\[\d+\] `)
)

type cause struct {
	name  string
	noise bool
	all   []string
}

var causes = []cause{
	{"", true, []string{" is deprecated"}},
	{"", true, []string{"Defined FakeDNS but haven't enabled FakeDNS destOverride"}},
	{"", true, []string{"core: Xray ", " started"}},
	{"direct_name", false, []string{"failed to resolve ip", "empty response"}},
	{"name_server", false, []string{"app/dns: failed to retrieve response"}},
	{"fake_address", false, []string{"no matching domain name in fake DNS"}},
	{"reserve", false, []string{"app/router: fallback to ["}},
	{"server_check", false, []string{"app/observatory", "error ping"}},
	{"no_network", false, []string{"app/observatory", "network is down"}},
	{"server_silent", false, []string{"failed to find an available destination"}},
}

func classify(text string) (string, bool) {
	for _, c := range causes {
		if !slices.ContainsFunc(c.all, func(s string) bool { return !strings.Contains(text, s) }) {
			return c.name, c.noise
		}
	}
	return "", false
}

type entry struct {
	line Line
	key  string
}

type Book struct {
	mu      sync.Mutex
	order   *list.List
	keys    map[string]*list.Element
	size    int
	now     func() time.Time
	hide    *strings.Replacer
	verbose bool
}

func New(size int, now func() time.Time) *Book {
	return &Book{order: list.New(), keys: map[string]*list.Element{}, size: size, now: now, hide: strings.NewReplacer()}
}

func (b *Book) Hide(words []string) {
	kept := make([]string, 0, len(words))
	for _, w := range words {
		if utf8.RuneCountInString(w) >= minWord {
			kept = append(kept, w)
		}
	}
	slices.SortFunc(kept, func(x, y string) int { return cmp.Compare(len(y), len(x)) })
	kept = slices.Compact(kept)
	pairs := make([]string, 0, 2*len(kept))
	for _, w := range kept {
		pairs = append(pairs, w, Hidden)
	}
	b.mu.Lock()
	b.hide = strings.NewReplacer(pairs...)
	b.mu.Unlock()
}

func (b *Book) Verbose(on bool) {
	b.mu.Lock()
	b.verbose = on
	b.mu.Unlock()
}

func (b *Book) Add(source, text string) {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.TrimSpace(text))
	name, noise := classify(text)
	b.mu.Lock()
	defer b.mu.Unlock()
	if noise && !b.verbose {
		return
	}
	text = uuid.ReplaceAllString(b.hide.Replace(text), Hidden)
	if len(text) > maxText {
		cut := maxText
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut] + "…"
	}
	at := b.now().Unix()
	key := source + "\x00" + name + "\x00"
	if name == "" {
		key += session.ReplaceAllString(text, "")
	}
	el, seen := b.keys[key]
	if source != "engine" {
		el = b.order.Back()
		seen = el != nil && el.Value.(*entry).key == key
	}
	if seen {
		l := &el.Value.(*entry).line
		l.Count = max(l.Count, 1) + 1
		l.Text, l.Last = text, at
		b.order.MoveToBack(el)
		return
	}
	b.keys[key] = b.order.PushBack(&entry{line: Line{At: at, Source: source, Cause: name, Text: text}, key: key})
	for b.order.Len() > b.size {
		first := b.order.Front()
		if k := first.Value.(*entry).key; b.keys[k] == first {
			delete(b.keys, k)
		}
		b.order.Remove(first)
	}
}

func (b *Book) Lines() []Line {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Line, 0, b.order.Len())
	for el := b.order.Front(); el != nil; el = el.Next() {
		out = append(out, el.Value.(*entry).line)
	}
	return out
}

func (b *Book) Clear() {
	b.mu.Lock()
	b.order.Init()
	clear(b.keys)
	b.mu.Unlock()
}
