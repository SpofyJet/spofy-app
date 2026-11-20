package host

import (
	"fmt"
	"sync"
	"time"

	"github.com/xtls/xray-core/proxy/tun"
	"golang.zx2c4.com/wintun"
)

const leftover = 5 * time.Second

type Wintun struct {
	Name string

	mu   sync.Mutex
	held *wintun.Adapter
}

func (w *Wintun) Raise() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.held != nil {
		return nil
	}
	for end := time.Now().Add(leftover); ; time.Sleep(50 * time.Millisecond) {
		other, err := wintun.OpenAdapter(w.Name)
		if err != nil {
			break
		}
		other.Close()
		if time.Now().After(end) {
			return fmt.Errorf("host: another adapter named %s did not go away", w.Name)
		}
	}
	held, err := wintun.CreateAdapter(w.Name, "Spofy", tun.AdapterGUID(w.Name))
	if err != nil {
		return fmt.Errorf("host: the adapter %s: %w", w.Name, err)
	}
	w.held = held
	return nil
}

func (w *Wintun) Lower() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.held != nil {
		w.held.Close()
		w.held = nil
	}
	return nil
}
