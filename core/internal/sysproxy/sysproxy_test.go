package sysproxy

import (
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"
)

type fake struct {
	conns    map[string]Settings
	rec      *Record
	broken   map[string]bool
	unread   map[string]bool
	notified int
	steps    []string
}

func (f *fake) Connections() ([]string, error) {
	out := []string{""}
	for c := range f.conns {
		if c != "" {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fake) Get(c string) (Settings, error) {
	if f.unread[c] {
		return Settings{}, errors.New("unreadable")
	}
	return f.conns[c], nil
}

func (f *fake) Put(c string, s Settings) error {
	f.steps = append(f.steps, "put "+c)
	if f.broken[c] {
		return errors.New("refused")
	}
	if f.rec == nil {
		panic("settings changed before the record was saved")
	}
	f.conns[c] = s
	return nil
}

func (f *fake) Notify() error {
	f.notified++
	return nil
}

func (f *fake) Load() (*Record, error) {
	if f.rec == nil {
		return nil, nil
	}
	r := &Record{Applied: f.rec.Applied, Previous: maps.Clone(f.rec.Previous)}
	return r, nil
}

func (f *fake) Save(r *Record) error {
	f.steps = append(f.steps, "save")
	f.rec = &Record{Applied: r.Applied, Previous: maps.Clone(r.Previous)}
	return nil
}

func (f *fake) Forget() error {
	f.steps = append(f.steps, "forget")
	f.rec = nil
	return nil
}

var (
	pac  = Settings{Flags: Direct | Detect | Script, Server: "10.1.2.3:3128", Bypass: "corp.example;<local>", Script: "http://pac.example/proxy.pac"}
	work = Settings{Flags: Direct | Manual, Server: "corp.example:8080"}
)

func person() *fake {
	return &fake{conns: map[string]Settings{"": pac, "Work VPN": work}, broken: map[string]bool{}, unread: map[string]bool{}}
}

func TestOnSavesWhatThePersonHadFirstAndOffBringsItBackExactly(t *testing.T) {
	f := person()
	if err := On(f, "127.0.0.1:41000"); err != nil {
		t.Fatal(err)
	}
	if f.steps[0] != "save" {
		t.Fatalf("the record must be on disk before anything changes: %v", f.steps)
	}
	for c, s := range f.conns {
		if s != Ours("127.0.0.1:41000") {
			t.Fatalf("%q = %+v", c, s)
		}
	}
	if err := Off(f); err != nil {
		t.Fatal(err)
	}
	if f.conns[""] != pac || f.conns["Work VPN"] != work || f.rec != nil || f.notified != 2 {
		t.Fatalf("after off: %+v record=%v notified=%d", f.conns, f.rec, f.notified)
	}
}

func TestAfterACrashANewAddressKeepsTheFirstSettings(t *testing.T) {
	f := person()
	On(f, "127.0.0.1:41000")
	if err := On(f, "127.0.0.1:52000"); err != nil {
		t.Fatal(err)
	}
	if f.rec.Previous[""] != pac || f.rec.Previous["Work VPN"] != work || f.conns[""].Server != "127.0.0.1:52000" {
		t.Fatalf("record = %+v, now %+v", f.rec, f.conns)
	}
	On(f, "127.0.0.1:52000")
	Off(f)
	if f.conns[""] != pac || f.conns["Work VPN"] != work {
		t.Fatalf("after off: %+v", f.conns)
	}
}

func TestWhatThePersonChangedMeanwhileStays(t *testing.T) {
	f := person()
	On(f, "127.0.0.1:41000")
	theirs := Settings{Flags: Direct | Manual, Server: "other.example:3128"}
	f.conns[""] = theirs
	if err := Off(f); err != nil {
		t.Fatal(err)
	}
	if f.conns[""] != theirs || f.conns["Work VPN"] != work || f.rec != nil {
		t.Fatalf("after off: %+v record=%v", f.conns, f.rec)
	}
}

func TestOffWithoutARecordTouchesNothing(t *testing.T) {
	f := person()
	if err := Off(f); err != nil {
		t.Fatal(err)
	}
	if len(f.steps) != 0 || f.notified != 0 || f.conns[""] != pac {
		t.Fatalf("steps=%v notified=%d", f.steps, f.notified)
	}
}

func TestOursLeftWithoutARecordIsNotTakenForThePersons(t *testing.T) {
	f := person()
	f.conns[""] = Ours("127.0.0.1:5555")
	On(f, "127.0.0.1:41000")
	Off(f)
	if f.conns[""] != (Settings{Flags: Direct}) || f.conns["Work VPN"] != work {
		t.Fatalf("after off: %+v", f.conns)
	}
	f = person()
	near := Settings{Flags: Direct | Manual, Server: "127.0.0.1:8888", Bypass: "<local>"}
	f.conns[""] = near
	On(f, "127.0.0.1:41000")
	Off(f)
	if f.conns[""] != near {
		t.Fatalf("a local proxy of the person's own is theirs: %+v", f.conns[""])
	}
}

func TestARestoreThatFailedIsKeptForTheNextTry(t *testing.T) {
	f := person()
	On(f, "127.0.0.1:41000")
	f.broken["Work VPN"] = true
	if err := Off(f); err == nil {
		t.Fatal("the failure is not reported")
	}
	if f.conns[""] != pac || f.rec == nil || !reflect.DeepEqual(f.rec.Previous, map[string]Settings{"Work VPN": work}) {
		t.Fatalf("after a half off: %+v record=%+v", f.conns, f.rec)
	}
	f.broken["Work VPN"] = false
	if err := Off(f); err != nil {
		t.Fatal(err)
	}
	if f.conns["Work VPN"] != work || f.rec != nil {
		t.Fatalf("after the second off: %+v record=%+v", f.conns, f.rec)
	}
}

func TestAnOnThatStoppedHalfWayLeavesNothingWrong(t *testing.T) {
	f := person()
	f.broken[""] = true
	if err := On(f, "127.0.0.1:41000"); err == nil {
		t.Fatal("the failure is not reported")
	}
	if f.conns[""] != pac || f.conns["Work VPN"] != Ours("127.0.0.1:41000") {
		t.Fatalf("after a half on: %+v", f.conns)
	}
	f.broken[""] = false
	Off(f)
	if f.conns[""] != pac || f.conns["Work VPN"] != work || f.rec != nil {
		t.Fatalf("after off: %+v record=%+v", f.conns, f.rec)
	}
}

func TestConnectionsThatComeAndGo(t *testing.T) {
	f := person()
	f.unread["Work VPN"] = true
	if err := On(f, "127.0.0.1:41000"); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.rec.Previous["Work VPN"]; ok || f.conns["Work VPN"] != work {
		t.Fatalf("an unreadable connection is left alone: %+v", f.rec)
	}
	f.unread["Work VPN"] = false
	home := Settings{Flags: Direct}
	f.conns["Home"] = home
	On(f, "127.0.0.1:41000")
	delete(f.conns, "Work VPN")
	if err := Off(f); err != nil {
		t.Fatal(err)
	}
	if f.conns["Home"] != home || f.conns[""] != pac || f.rec != nil {
		t.Fatalf("after off: %+v record=%+v", f.conns, f.rec)
	}
}

func TestTheBypassListKeepsTheLocalNetworkDirect(t *testing.T) {
	for _, want := range []string{"<local>", "localhost", "127.*", "10.*", "172.16.*", "172.31.*", "192.168.*", "169.254.*", "*.local"} {
		found := false
		for _, e := range strings.Split(Bypass, ";") {
			found = found || e == want
		}
		if !found {
			t.Errorf("%s is not in the bypass list", want)
		}
	}
	if n := len(strings.Split(Bypass, ";")); n != 24 {
		t.Errorf("%d entries", n)
	}
}
