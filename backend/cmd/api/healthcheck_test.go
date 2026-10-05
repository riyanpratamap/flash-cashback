package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthcheckURL(t *testing.T) {
	tests := map[string]string{
		":8080":          "http://127.0.0.1:8080/v1/healthz",
		"0.0.0.0:8080":   "http://127.0.0.1:8080/v1/healthz",
		"127.0.0.1:9000": "http://127.0.0.1:9000/v1/healthz",
		"example.com:81": "http://127.0.0.1:81/v1/healthz",
	}
	for addr, want := range tests {
		if got := healthcheckURL(addr); got != want {
			t.Errorf("healthcheckURL(%q) = %q, want %q", addr, got, want)
		}
	}
}

func TestHealthcheckExitCode(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))
	url := srv.URL

	if got := healthcheck(url, time.Second); got != 0 {
		t.Errorf("200: exit %d, want 0", got)
	}
	status = http.StatusServiceUnavailable
	if got := healthcheck(url, time.Second); got != 1 {
		t.Errorf("503: exit %d, want 1", got)
	}
	srv.Close()
	if got := healthcheck(url, time.Second); got != 1 {
		t.Errorf("closed port: exit %d, want 1", got)
	}
}
