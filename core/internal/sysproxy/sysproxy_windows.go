package sysproxy

import (
	"encoding/json"
	"errors"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	optionPerConnection        = 75
	optionSettingsChanged      = 39
	optionRefresh              = 37
	optionProxySettingsChanged = 95

	perConnFlags   = 1
	perConnServer  = 2
	perConnBypass  = 3
	perConnScript  = 4
	perConnFlagsUI = 10

	rasBufferTooSmall = 603

	recordKey   = `Software\Spofy`
	recordValue = "SystemProxy"
	lockName    = `Local\app.spofy.proxy`
	lockWait    = 10 * time.Second
)

var (
	wininet             = windows.NewLazySystemDLL("wininet.dll")
	rasapi              = windows.NewLazySystemDLL("rasapi32.dll")
	kernel32            = windows.NewLazySystemDLL("kernel32.dll")
	internetQueryOption = wininet.NewProc("InternetQueryOptionW")
	internetSetOption   = wininet.NewProc("InternetSetOptionW")
	rasEnumEntries      = rasapi.NewProc("RasEnumEntriesW")
	globalFree          = kernel32.NewProc("GlobalFree")
)

type option struct {
	ID    uint32
	Value *uint16
}

var _ = [1]byte{}[unsafe.Sizeof(option{})-16]

type optionList struct {
	Size       uint32
	Connection *uint16
	Count      uint32
	Error      uint32
	Options    *option
}

type rasEntryName struct {
	Size      uint32
	Name      [257]uint16
	Flags     uint32
	Phonebook [261]uint16
}

func (o *option) dword() *uint32 {
	return (*uint32)(unsafe.Pointer(&o.Value))
}

type WinINet struct{}

func (WinINet) Connections() ([]string, error) {
	out := []string{""}
	if rasEnumEntries.Find() != nil {
		return out, nil
	}
	entries := make([]rasEntryName, 1)
	for range 2 {
		entries[0].Size = uint32(unsafe.Sizeof(entries[0]))
		size, count := uint32(len(entries))*entries[0].Size, uint32(0)
		r, _, _ := rasEnumEntries.Call(0, 0, uintptr(unsafe.Pointer(&entries[0])), uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)))
		if r == rasBufferTooSmall {
			entries = make([]rasEntryName, size/entries[0].Size+1)
			continue
		}
		if r != 0 {
			return out, nil
		}
		for _, e := range entries[:count] {
			if name := windows.UTF16ToString(e.Name[:]); name != "" {
				out = append(out, name)
			}
		}
		return out, nil
	}
	return out, nil
}

func call(set bool, conn string, opts []option) error {
	list := optionList{Count: uint32(len(opts)), Options: &opts[0]}
	list.Size = uint32(unsafe.Sizeof(list))
	if conn != "" {
		name, err := windows.UTF16PtrFromString(conn)
		if err != nil {
			return err
		}
		list.Connection = name
	}
	size := list.Size
	var r uintptr
	var err error
	if set {
		r, _, err = internetSetOption.Call(0, optionPerConnection, uintptr(unsafe.Pointer(&list)), uintptr(size))
	} else {
		r, _, err = internetQueryOption.Call(0, optionPerConnection, uintptr(unsafe.Pointer(&list)), uintptr(unsafe.Pointer(&size)))
	}
	runtime.KeepAlive(opts)
	runtime.KeepAlive(&list)
	if r == 0 {
		return err
	}
	return nil
}

func take(p *uint16) string {
	if p == nil {
		return ""
	}
	s := windows.UTF16PtrToString(p)
	globalFree.Call(uintptr(unsafe.Pointer(p)))
	return s
}

func (WinINet) Get(conn string) (Settings, error) {
	var last error
	for _, flags := range []uint32{perConnFlagsUI, perConnFlags} {
		opts := []option{{ID: flags}, {ID: perConnServer}, {ID: perConnBypass}, {ID: perConnScript}}
		if err := call(false, conn, opts); err != nil {
			last = err
			continue
		}
		return Settings{Flags: *opts[0].dword(), Server: take(opts[1].Value), Bypass: take(opts[2].Value), Script: take(opts[3].Value)}, nil
	}
	return Settings{}, last
}

func text(s string) (*uint16, error) {
	if s == "" {
		return nil, nil
	}
	return windows.UTF16PtrFromString(s)
}

func (WinINet) Put(conn string, s Settings) error {
	var strs [3]*uint16
	for i, v := range []string{s.Server, s.Bypass, s.Script} {
		p, err := text(v)
		if err != nil {
			return err
		}
		strs[i] = p
	}
	err := errors.New("no flags option was taken")
	for _, flags := range [][]uint32{{perConnFlagsUI, perConnFlags}, {perConnFlags}} {
		opts := make([]option, 0, 5)
		for _, f := range flags {
			o := option{ID: f}
			*o.dword() = s.Flags
			opts = append(opts, o)
		}
		opts = append(opts, option{ID: perConnServer, Value: strs[0]}, option{ID: perConnBypass, Value: strs[1]}, option{ID: perConnScript, Value: strs[2]})
		if err = call(true, conn, opts); err == nil {
			return nil
		}
	}
	return err
}

func (WinINet) Notify() error {
	var errs []error
	for _, o := range []uintptr{optionProxySettingsChanged, optionSettingsChanged, optionRefresh} {
		if r, _, err := internetSetOption.Call(0, o, 0, 0); r == 0 {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (WinINet) Load() (*Record, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, recordKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(recordValue)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r Record
	if json.Unmarshal([]byte(v), &r) != nil || r.Applied == "" {
		return nil, nil
	}
	return &r, nil
}

func (WinINet) Save(r *Record) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, recordKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(recordValue, string(b))
}

func (WinINet) Forget() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, recordKey, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(recordValue); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

func locked(work func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	name, _ := windows.UTF16PtrFromString(lockName)
	h, err := windows.CreateMutex(nil, false, name)
	if h == 0 {
		return errors.Join(work(), err)
	}
	defer windows.CloseHandle(h)
	ev, _ := windows.WaitForSingleObject(h, uint32(lockWait/time.Millisecond))
	if ev == windows.WAIT_OBJECT_0 || ev == windows.WAIT_ABANDONED {
		defer windows.ReleaseMutex(h)
	}
	return work()
}

func Apply(address string) error {
	return locked(func() error { return On(WinINet{}, address) })
}

func Restore() error {
	return locked(func() error { return Off(WinINet{}) })
}
