package main

import (
	"bytes"
	"context"
	"net/http"
	"time"
)

var defaultHTTPClient = &http.Client{Timeout: 60 * time.Second}

func newHTTPRequest(ctx context.Context, method, url string, body []byte) (*http.Request, error) {
	var r *http.Request
	var err error
	if body != nil {
		r, err = http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	} else {
		r, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	return r, err
}
