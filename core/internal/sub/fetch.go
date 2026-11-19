package sub

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

var ErrNotModified = errors.New("sub: not modified")

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
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotModified:
		if v := resp.Header.Get("Subscription-Userinfo"); v != "" {
			return &Profile{Usage: usage(v)}, ErrNotModified
		}
		return nil, ErrNotModified
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
	return Parse(body, resp.Header)
}
