package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/SpofyJet/spofy-app/core/internal/tray"
)

const (
	pipeAccess = 0x0012008B
	maxLine    = 1 << 20
	writeWait  = 5000
	busyWait   = 2 * time.Second
	firstWait  = 250 * time.Millisecond
	longWait   = 2 * time.Second
	steady     = time.Second
)

var orphaned = 5 * time.Second

var (
	errStranger = errors.New("the pipe belongs to someone other than the system")
	errIcon     = errors.New("the icon file is broken")
)

type shell struct {
	hwnd     windows.HWND
	taskbar  uint32
	icons    [4]windows.Handle
	large    windows.Handle
	added    bool
	view     tray.View
	stalled  bool
	quitting uint64
	wake     func()
	restore  func()

	mu       sync.Mutex
	model    tray.Model
	out      chan []byte
	notes    []tray.Note
	answered []uint64
	next     atomic.Uint64
}

type op struct {
	ov *windows.Overlapped
	n  uint32
}

type pipe struct {
	h      windows.Handle
	rd, wr op
	mu     sync.Mutex
	closed bool
	ops    sync.WaitGroup
	once   sync.Once
}

func dial(ctx context.Context, path string) (*pipe, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	until := time.Now().Add(busyWait)
	for {
		h, err := windows.CreateFile(name, pipeAccess, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED|windows.SECURITY_SQOS_PRESENT|windows.SECURITY_IDENTIFICATION, 0)
		if errors.Is(err, windows.ERROR_PIPE_BUSY) && time.Now().Before(until) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Millisecond):
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		if !trusted(h) {
			windows.CloseHandle(h)
			return nil, errStranger
		}
		p := &pipe{h: h}
		for _, o := range []*op{&p.rd, &p.wr} {
			ev, err := windows.CreateEvent(nil, 1, 0, nil)
			if err != nil {
				p.Close()
				return nil, err
			}
			o.ov = &windows.Overlapped{HEvent: ev}
		}
		return p, nil
	}
}

func (p *pipe) do(o *op, write bool, b []byte, limit uint32) (int, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return 0, net.ErrClosed
	}
	p.ops.Add(1)
	defer p.ops.Done()
	var err error
	if write {
		err = windows.WriteFile(p.h, b, &o.n, o.ov)
	} else {
		err = windows.ReadFile(p.h, b, &o.n, o.ov)
	}
	p.mu.Unlock()
	if err != nil && !errors.Is(err, windows.ERROR_IO_PENDING) {
		return 0, ended(err)
	}
	if ev, _ := windows.WaitForSingleObject(o.ov.HEvent, limit); ev == uint32(windows.WAIT_TIMEOUT) {
		windows.CancelIoEx(p.h, o.ov)
	}
	var n uint32
	if err := windows.GetOverlappedResult(p.h, o.ov, &n, true); err != nil {
		return int(n), ended(err)
	}
	return int(n), nil
}

func ended(err error) error {
	switch {
	case errors.Is(err, windows.ERROR_BROKEN_PIPE), errors.Is(err, windows.ERROR_PIPE_NOT_CONNECTED), errors.Is(err, windows.ERROR_NO_DATA):
		return io.EOF
	case errors.Is(err, windows.ERROR_OPERATION_ABORTED):
		return net.ErrClosed
	}
	return err
}

func (p *pipe) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	return p.do(&p.rd, false, b, windows.INFINITE)
}

func (p *pipe) Write(b []byte) (int, error) {
	done := 0
	for done < len(b) {
		n, err := p.do(&p.wr, true, b[done:], writeWait)
		done += n
		if err != nil {
			return done, err
		}
		if n == 0 {
			return done, io.ErrShortWrite
		}
	}
	return done, nil
}

func (p *pipe) Close() error {
	p.once.Do(func() {
		p.mu.Lock()
		p.closed = true
		windows.CancelIoEx(p.h, nil)
		p.mu.Unlock()
		p.ops.Wait()
		windows.CloseHandle(p.h)
		for _, o := range []*op{&p.rd, &p.wr} {
			if o.ov != nil {
				windows.CloseHandle(o.ov.HEvent)
			}
		}
	})
	return nil
}

func trusted(h windows.Handle) bool {
	sd, err := windows.GetSecurityInfo(h, windows.SE_KERNEL_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	owner, _, err := sd.Owner()
	return err == nil && (owner.IsWellKnown(windows.WinLocalSystemSid) || owner.IsWellKnown(windows.WinBuiltinAdministratorsSid))
}

func (s *shell) serve(ctx context.Context, path string) {
	wait := time.Duration(0)
	lost, given := time.Now(), false
	for {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		wait = min(max(wait*2, firstWait), longWait)
		f, err := dial(ctx, path)
		if err != nil {
			if !given && s.restore != nil && time.Since(lost) >= orphaned {
				given = true
				s.restore()
			}
			continue
		}
		began := time.Now()
		s.session(ctx, f)
		lost, given = time.Now(), false
		if time.Since(began) > steady {
			wait = 0
		}
	}
}

func (s *shell) session(ctx context.Context, f *pipe) {
	out := make(chan []byte, 16)
	hello := s.next.Add(1)
	out <- request(hello, "hello")
	s.mu.Lock()
	s.model.Link(hello)
	s.out = out
	s.mu.Unlock()
	stop := context.AfterFunc(ctx, func() { f.Close() })
	written := make(chan struct{})
	go func() {
		defer close(written)
		for b := range out {
			if _, err := f.Write(b); err != nil {
				f.Close()
				return
			}
		}
	}()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	for sc.Scan() {
		s.mu.Lock()
		r, err := s.model.Read(sc.Bytes())
		if r.Note != nil {
			s.notes = append(s.notes, *r.Note)
		}
		if r.Answered != 0 {
			s.answered = append(s.answered, r.Answered)
		}
		s.mu.Unlock()
		if err != nil {
			break
		}
		if r.Changed || r.Note != nil || r.Answered != 0 {
			s.wake()
		}
	}
	stop()
	s.mu.Lock()
	s.out = nil
	s.model.Lost()
	s.mu.Unlock()
	close(out)
	f.Close()
	<-written
	s.wake()
}

func (s *shell) ask(op string, a tray.Action) uint64 {
	id := s.next.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.out == nil || !s.model.Linked() {
		return 0
	}
	select {
	case s.out <- request(id, op):
		s.model.Ask(id, a)
		return id
	default:
		return 0
	}
}

func (s *shell) set(key, value string) {
	b, _ := json.Marshal(struct {
		ID    uint64 `json:"id"`
		Op    string `json:"op"`
		Key   string `json:"key"`
		Value string `json:"value"`
	}{s.next.Add(1), "set", key, value})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.out == nil || !s.model.Linked() {
		return
	}
	select {
	case s.out <- append(b, '\n'):
	default:
	}
}

func request(id uint64, op string) []byte {
	b, _ := json.Marshal(struct {
		ID uint64 `json:"id"`
		Op string `json:"op"`
	}{id, op})
	return append(b, '\n')
}

func makeIcon(ico []byte, size int) (windows.Handle, error) {
	if len(ico) < 6 || binary.LittleEndian.Uint16(ico[2:]) != 1 {
		return 0, errIcon
	}
	count := int(binary.LittleEndian.Uint16(ico[4:]))
	if count == 0 || len(ico) < 6+16*count {
		return 0, errIcon
	}
	pick, at := -1, 0
	for i := range count {
		w := int(ico[6+16*i])
		if w == 0 {
			w = 256
		}
		if pick < 0 || (at < size && w > at) || (w >= size && w < at) {
			pick, at = i, w
		}
	}
	entry := ico[6+16*pick:]
	n := uint64(binary.LittleEndian.Uint32(entry[8:]))
	off := uint64(binary.LittleEndian.Uint32(entry[12:]))
	if n == 0 || off+n > uint64(len(ico)) {
		return 0, errIcon
	}
	image := ico[off : off+n]
	h, _, _ := createIconFromResourceEx.Call(uintptr(unsafe.Pointer(&image[0])), uintptr(n), 1, 0x00030000, uintptr(size), uintptr(size), 0)
	if h == 0 {
		return 0, errIcon
	}
	return windows.Handle(h), nil
}
