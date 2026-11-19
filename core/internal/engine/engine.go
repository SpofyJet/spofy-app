package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	applog "github.com/xtls/xray-core/app/log"
	clog "github.com/xtls/xray-core/common/log"
	"github.com/xtls/xray-core/common/platform"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/stats"
	"github.com/xtls/xray-core/infra/conf/serial"
	_ "github.com/xtls/xray-core/main/distro/all"
)

type Code string

const (
	CodeConfig     Code = "config_invalid"
	CodePermission Code = "tun_permission"
	CodeStart      Code = "start_failed"
	CodeBusy       Code = "engine_busy"
)

type Error struct {
	Code Code
	Err  error
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

type Traffic struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
	Back int64 `json:"back,omitempty"`
}

type Instance struct {
	inst  *core.Instance
	stats stats.Manager
	far   []string
	once  sync.Once
}

var (
	sink    atomic.Pointer[func(string)]
	running atomic.Bool
)

func SetLogSink(f func(string)) {
	if f == nil {
		sink.Store(nil)
		return
	}
	sink.Store(&f)
}

type handler struct{}

func (handler) Handle(msg clog.Message) {
	if f := sink.Load(); f != nil {
		(*f)(msg.String())
	}
}

func init() {
	applog.RegisterHandlerCreator(applog.LogType_Console, func(applog.LogType, applog.HandlerCreatorOptions) (clog.Handler, error) {
		return handler{}, nil
	})
	clog.RegisterHandler(handler{})
}

func Validate(cfg []byte) error {
	clog.RegisterHandler(handler{})
	c, err := serial.LoadJSONConfig(bytes.NewReader(cfg))
	if err != nil {
		return &Error{Code: CodeConfig, Err: err}
	}
	if len(c.Outbound) == 0 {
		return &Error{Code: CodeConfig, Err: errors.New("no outbounds")}
	}
	return nil
}

func Start(cfg []byte) (*Instance, error) {
	return StartOn(cfg, -1)
}

func StartOn(cfg []byte, tun int) (*Instance, error) {
	if !running.CompareAndSwap(false, true) {
		if tun >= 0 {
			shut(tun)
		}
		return nil, &Error{Code: CodeBusy, Err: errors.New("another instance is running in this process")}
	}
	if tun >= 0 {
		os.Setenv(platform.TunFdKey, strconv.Itoa(tun))
	}
	i, err := start(cfg)
	if tun >= 0 {
		os.Unsetenv(platform.TunFdKey)
		if runtime.GOOS == "android" {
			shut(tun)
		}
	}
	if err != nil {
		running.Store(false)
	}
	return i, err
}

func start(cfg []byte) (*Instance, error) {
	clog.RegisterHandler(handler{})
	c, err := serial.LoadJSONConfig(bytes.NewReader(cfg))
	if err != nil {
		return nil, &Error{Code: CodeConfig, Err: err}
	}
	inst, err := core.New(c)
	if err != nil {
		return nil, classify(err, CodeConfig)
	}
	if err := inst.Start(); err != nil {
		inst.Close()
		return nil, classify(err, CodeStart)
	}
	i := &Instance{inst: inst, far: far(cfg)}
	if m, ok := inst.GetFeature(stats.ManagerType()).(stats.Manager); ok {
		i.stats = m
	}
	return i, nil
}

func far(cfg []byte) []string {
	var shape struct {
		Outbounds []struct {
			Tag      string `json:"tag"`
			Protocol string `json:"protocol"`
		} `json:"outbounds"`
	}
	if json.Unmarshal(cfg, &shape) != nil {
		return nil
	}
	var tags []string
	for _, o := range shape.Outbounds {
		switch o.Protocol {
		case "freedom", "blackhole", "dns", "loopback":
		default:
			if o.Tag != "" {
				tags = append(tags, o.Tag)
			}
		}
	}
	return tags
}

func (i *Instance) Close() error {
	var err error
	i.once.Do(func() {
		err = i.inst.Close()
		clog.RegisterHandler(handler{})
		running.Store(false)
	})
	return err
}

func (i *Instance) Traffic(tag string) Traffic {
	if i.stats == nil {
		return Traffic{}
	}
	read := func(side, tag, dir string) int64 {
		if c := i.stats.GetCounter(side + ">>>" + tag + ">>>traffic>>>" + dir); c != nil {
			return c.Value()
		}
		return 0
	}
	t := Traffic{Up: read("inbound", tag, "uplink"), Down: read("inbound", tag, "downlink")}
	for _, out := range i.far {
		t.Back += read("outbound", out, "downlink")
	}
	if len(i.far) == 0 {
		t.Back = t.Down
	}
	return t
}

func classify(err error, fallback Code) *Error {
	text := strings.ToLower(err.Error())
	denied := errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES)
	for _, s := range []string{"operation not permitted", "permission denied", "access is denied"} {
		denied = denied || strings.Contains(text, s)
	}
	if denied {
		return &Error{Code: CodePermission, Err: err}
	}
	return &Error{Code: fallback, Err: err}
}
