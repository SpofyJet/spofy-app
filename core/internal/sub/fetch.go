package sub

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

var (
	ErrNotModified = errors.New("sub: not modified")
	ErrDowngrade   = errors.New("sub: redirect away from https")
)

type StatusError struct {
	Code int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("sub: http status %d", e.Code)
}

type Device struct {
	HWID       string
	OS         string
	OSVersion  string
	Model      string
	AppVersion string
}

func (d Device) UserAgent() string {
	return fmt.Sprintf("Spofy/%s (%s %s; %s)", d.AppVersion, d.OS, d.OSVersion, d.Model)
}

const maxBody = 8 << 20

func Fetch(ctx context.Context, c *http.Client, url string, d Device, etag string) (*Profile, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", d.UserAgent())
	req.Header.Set("Accept", "application/json")
	if d.HWID != "" {
		req.Header.Set("X-Hwid", d.HWID)
		req.Header.Set("X-Device-Os", d.OS)
		req.Header.Set("X-Ver-Os", d.OSVersion)
		req.Header.Set("X-Device-Model", d.Model)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	permanent, hops := true, 0
	follow := *c
	follow.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("sub: too many redirects")
		}
		if via[0].URL.Scheme == "https" && next.URL.Scheme != "https" {
			return ErrDowngrade
		}
		hops++
		code := next.Response.StatusCode
		permanent = permanent && (code == http.StatusMovedPermanently || code == http.StatusPermanentRedirect)
		return nil
	}
	resp, err := follow.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	moved := ""
	if permanent && hops > 0 {
		moved = resp.Request.URL.String()
	}
	switch {
	case resp.StatusCode == http.StatusNotModified:
		p := &Profile{Moved: moved}
		if v := resp.Header.Get("Subscription-Userinfo"); v != "" {
			p.Usage, p.Counted = usage(v), true
		}
		return p, ErrNotModified
	case resp.StatusCode != http.StatusOK:
		return nil, &StatusError{Code: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBody {
		return nil, ErrFormat
	}
	p, err := Parse(body, resp.Header)
	if p != nil {
		p.Moved = moved
	}
	return p, err
}
