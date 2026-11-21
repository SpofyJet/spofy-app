package engine

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
)

const (
	CodeFetch Code = "fetch_failed"
	FetchMax       = 2 << 20
)

var roots *x509.CertPool

type Fetched struct {
	Status int                 `json:"status"`
	Header map[string][]string `json:"header,omitempty"`
	Body   []byte              `json:"body,omitempty"`
}

func (i *Instance) Fetch(ctx context.Context, target, tag string, head map[string]string) (*Fetched, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil || req.URL.Scheme != "https" {
		return nil, &Error{Code: CodeConfig, Err: errors.New("only an https address is carried")}
	}
	for k, v := range head {
		req.Header.Set(k, v)
	}
	tr := i.transport(tag)
	tr.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, &Error{Code: CodeFetch, Err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, FetchMax+1))
	if err != nil {
		return nil, &Error{Code: CodeFetch, Err: err}
	}
	if len(body) > FetchMax {
		return nil, &Error{Code: CodeFetch, Err: errors.New("the answer is larger than the line can carry")}
	}
	return &Fetched{Status: resp.StatusCode, Header: resp.Header, Body: body}, nil
}
