package tray

import (
	"encoding/json"
	"strings"
	"unicode/utf16"
)

const Protocol = 1

type Icon int

const (
	Off Icon = iota
	Busy
	On
	Alert
)

type Action int

const (
	Nothing Action = iota
	Open
	Connect
	Disconnect
)

type Item struct {
	Label  string
	Action Action
	Grey   bool
}

type View struct {
	Icon    Icon
	Tip     string
	Status  string
	Toggle  Item
	Quit    Item
	Stalled bool
}

type Note struct {
	Title string
	Text  string
}

type Result struct {
	Changed  bool
	Note     *Note
	Answered uint64
}

type connection struct {
	State  string `json:"state"`
	Server string `json:"server"`
	Code   string `json:"code"`
}

type server struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type subscription struct {
	Stop string `json:"stop"`
}

type snapshot struct {
	Protocol     int           `json:"protocol"`
	Connection   connection    `json:"connection"`
	Subscription *subscription `json:"subscription"`
	Servers      []server      `json:"servers"`
}

type message struct {
	Re           uint64          `json:"re"`
	OK           bool            `json:"ok"`
	Code         string          `json:"code"`
	Ev           string          `json:"ev"`
	Snapshot     *snapshot       `json:"snapshot"`
	Connection   *connection     `json:"connection"`
	Subscription json.RawMessage `json:"subscription"`
	Servers      []server        `json:"servers"`
}

type Model struct {
	hello   uint64
	linked  bool
	stale   bool
	conn    connection
	sub     *subscription
	servers []server
	asked   map[uint64]Action
	told    bool
}

func (m *Model) Link(hello uint64) {
	*m = Model{hello: hello, asked: map[uint64]Action{}}
}

func (m *Model) Lost() {
	*m = Model{}
}

func (m *Model) Linked() bool {
	return m.linked
}

func (m *Model) Ask(id uint64, a Action) {
	if m.asked != nil {
		m.asked[id] = a
	}
}

func (m *Model) Read(line []byte) (Result, error) {
	var msg message
	if err := json.Unmarshal(line, &msg); err != nil {
		return Result{}, err
	}
	if msg.Ev != "" {
		if !m.linked {
			return Result{}, nil
		}
		return m.event(msg), nil
	}
	if msg.Re == 0 {
		return Result{}, nil
	}
	if msg.Re == m.hello && m.hello != 0 {
		m.hello = 0
		if !msg.OK || msg.Snapshot == nil {
			return Result{}, nil
		}
		if msg.Snapshot.Protocol != Protocol {
			m.stale = true
			return Result{Changed: true}, nil
		}
		m.linked = true
		m.conn = msg.Snapshot.Connection
		m.sub = msg.Snapshot.Subscription
		m.servers = msg.Snapshot.Servers
		return Result{Changed: true}, nil
	}
	action, ok := m.asked[msg.Re]
	if !ok {
		return Result{}, nil
	}
	delete(m.asked, msg.Re)
	r := Result{Answered: msg.Re}
	if !msg.OK && action == Connect && !(m.conn.State == "failed" && m.conn.Code == msg.Code) {
		r.Note = &Note{Title: Title("failed"), Text: Reason(msg.Code)}
		m.told = m.conn.State == "stalled"
	}
	return r, nil
}

func (m *Model) event(msg message) Result {
	switch msg.Ev {
	case "connection":
		if msg.Connection == nil {
			return Result{}
		}
		was := m.conn.State
		m.conn = *msg.Connection
		if m.conn.State != "stalled" {
			m.told = false
		}
		r := Result{Changed: true}
		if m.conn.State == "failed" && was != "failed" && m.conn.Code != "" {
			r.Note = &Note{Title: Title("failed"), Text: Reason(m.conn.Code)}
		}
		return r
	case "subscription":
		var s *subscription
		if json.Unmarshal(msg.Subscription, &s) != nil {
			return Result{}
		}
		m.sub = s
		return Result{Changed: true}
	case "servers":
		m.servers = msg.Servers
		return Result{Changed: true}
	}
	return Result{}
}

func (m *Model) StalledNote() *Note {
	if !m.linked || m.conn.State != "stalled" || m.told {
		return nil
	}
	m.told = true
	return &Note{Title: Title("stalled"), Text: Hint("stalled")}
}

func (m *Model) View() View {
	quit := Item{Label: textQuit}
	if !m.linked {
		status := textLost
		if m.stale {
			status = textStale
		}
		return View{Icon: Off, Tip: tip(status), Status: status, Toggle: Item{Label: textConnect, Grey: true}, Quit: quit}
	}
	state := m.conn.State
	v := View{Status: Title(state), Stalled: state == "stalled"}
	switch state {
	case "connected":
		v.Icon = On
	case "measuring", "connecting", "switching":
		v.Icon = Busy
	case "stalled", "offline":
		v.Icon = Alert
	default:
		v.Icon = Off
	}
	if (state == "connected" || state == "stalled") && m.conn.Server != "" {
		v.Status += " · " + m.name(m.conn.Server)
	}
	switch state {
	case "idle", "failed":
		switch {
		case m.sub == nil:
			v.Toggle = Item{Label: textAddSubscription, Action: Open}
		case m.sub.Stop != "":
			v.Status = StopTitle(m.sub.Stop)
			v.Toggle = Item{Label: StopAction(m.sub.Stop), Action: Open}
		default:
			v.Toggle = Item{Label: textConnect, Action: Connect}
		}
	case "measuring", "connecting", "switching":
		v.Toggle = Item{Label: textStop, Action: Disconnect}
		quit = Item{Label: textQuitOff, Action: Disconnect}
	default:
		v.Toggle = Item{Label: textDisconnect, Action: Disconnect}
		quit = Item{Label: textQuitOff, Action: Disconnect}
	}
	v.Quit = quit
	v.Tip = tip(v.Status)
	return v
}

func (m *Model) name(id string) string {
	for _, s := range m.servers {
		if s.ID == id {
			if label := strings.TrimSpace(s.Label); label != "" {
				return label
			}
			break
		}
	}
	return id
}

func tip(status string) string {
	return Clip(textApp+"\n"+status, 127)
}

func Clip(s string, units int) string {
	n := 0
	for i, r := range s {
		w := utf16.RuneLen(r)
		if n+w > units {
			return s[:i]
		}
		n += w
	}
	return s
}
