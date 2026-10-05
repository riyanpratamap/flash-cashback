package main

import (
	"context"
	"net"
	"net/http"
	"time"
)

// healthcheckURL points at the local server: only the port of addr matters,
// because the check always runs inside the same container.
func healthcheckURL(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		port = "8080"
	}
	return "http://" + net.JoinHostPort("127.0.0.1", port) + "/v1/healthz"
}

// healthcheck returns the process exit code: 0 on HTTP 200, otherwise 1.
func healthcheck(url string, timeout time.Duration) int {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
