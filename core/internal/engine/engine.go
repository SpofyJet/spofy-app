package engine

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"syscall"

	applog "github.com/xtls/xray-core/app/log"
	clog "github.com/xtls/xray-core/common/log"
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
}

type Instance struct {
	inst  *core.Instance
	stats stats.Manager
}

var sink atomic.Pointer[func(string)]

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
	i := &Instance{inst: inst}
	if m, ok := inst.GetFeature(stats.ManagerType()).(stats.Manager); ok {
		i.stats = m
	}
	return i, nil
}

func (i *Instance) Close() error {
	err := i.inst.Close()
	clog.RegisterHandler(handler{})
	return err
}

func (i *Instance) Traffic(tag string) Traffic {
	if i.stats == nil {
		return Traffic{}
	}
	read := func(dir string) int64 {
		if c := i.stats.GetCounter("inbound>>>" + tag + ">>>traffic>>>" + dir); c != nil {
			return c.Value()
		}
		return 0
	}
	return Traffic{Up: read("uplink"), Down: read("downlink")}
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
