package logbook

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func clock() func() time.Time {
	at := time.Unix(1795000000, 0)
	return func() time.Time { return at }
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
