package engine

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/core"
)

const (
	CodeProbe Code = "probe_failed"
	CodeIdle  Code = "not_started"
)

var ProbeURLs = []string{"http://www.gstatic.com/generate_204", "http://cp.cloudflare.com/generate_204"}

func (i *Instance) Probe(ctx context.Context, target, tag string) (time.Duration, error) {
	if target != "" {
		return i.probe(ctx, target, tag)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		took time.Duration
		err  error
	}
	out := make(chan result, len(ProbeURLs))
	for _, u := range ProbeURLs {
		go func() {
			took, err := i.probe(ctx, u, tag)
			out <- result{took, err}
		}()
	}
	best := result{err: &Error{Code: CodeProbe, Err: errors.New("no probe target")}}
	for range ProbeURLs {
		r := <-out
		if r.err == nil && best.err != nil {
			best = r
			cancel()
		} else if best.err != nil {
			best = r
		}
	}
	return best.took, best.err
}

func (i *Instance) probe(ctx context.Context, target, tag string) (time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, &Error{Code: CodeConfig, Err: err}
	}
	tr := &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			n, err := strconv.ParseUint(port, 10, 16)
			if err != nil {
				return nil, err
			}
			if tag != "" {
				ctx = session.SetForcedOutboundTagToContext(ctx, tag)
			}
			return core.Dial(ctx, i.inst, xnet.TCPDestination(xnet.ParseAddress(host), xnet.Port(n)))
		},
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	began := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, &Error{Code: CodeProbe, Err: err}
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return 0, &Error{Code: CodeProbe, Err: errors.New("unexpected status " + strconv.Itoa(resp.StatusCode))}
	}
	return time.Since(began), nil
}

var measuring sync.Mutex

func Measure(ctx context.Context, cfg []byte, target, tag string) (time.Duration, error) {
	measuring.Lock()
	defer measuring.Unlock()
	inst, err := Start(cfg)
	if err != nil {
		return 0, err
	}
	defer inst.Close()
	return inst.Probe(ctx, target, tag)
}
