package logbook

import (
	"cmp"
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
	Source string `json:"source"`
	Text   string `json:"text"`
}

const (
	Hidden  = "•••"
	maxText = 512
	minWord = 4
)

var uuid = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

type Book struct {
	mu    sync.Mutex
	lines []Line
	head  int
	size  int
	now   func() time.Time
	hide  *strings.Replacer
}

func New(size int, now func() time.Time) *Book {
	return &Book{lines: make([]Line, 0, size), size: size, now: now, hide: strings.NewReplacer()}
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

func (b *Book) Add(source, text string) {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.TrimSpace(text))
	b.mu.Lock()
	defer b.mu.Unlock()
	text = uuid.ReplaceAllString(b.hide.Replace(text), Hidden)
	if len(text) > maxText {
		cut := maxText
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut] + "…"
	}
	line := Line{At: b.now().Unix(), Source: source, Text: text}
	if len(b.lines) < b.size {
		b.lines = append(b.lines, line)
		return
	}
	b.lines[b.head] = line
	b.head = (b.head + 1) % b.size
}

func (b *Book) Lines() []Line {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append(append(make([]Line, 0, len(b.lines)), b.lines[b.head:]...), b.lines[:b.head]...)
}

func (b *Book) Clear() {
	b.mu.Lock()
	b.lines, b.head = b.lines[:0], 0
	b.mu.Unlock()
}
