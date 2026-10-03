package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsLoopbackAddr(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:3333":     true,
		"localhost:3333":     true,
		"[::1]:3333":         true,
		"127.5.5.5:80":       true,
		"0.0.0.0:3333":       false,
		"192.168.1.5:3333":   false,
		"[::]:3333":          false,
		"example.local:3333": false,
		"3333":               false,
	}
	for addr, want := range cases {
		if got := isLoopbackAddr(addr); got != want {
			t.Errorf("isLoopbackAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestNormalizeListenAddr(t *testing.T) {
	cases := map[string]string{
		"":             "",
		":3333":        "127.0.0.1:3333",
		"0.0.0.0:3333": "0.0.0.0:3333",
	}
	for in, want := range cases {
		if got := normalizeListenAddr(in); got != want {
			t.Errorf("normalizeListenAddr(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBearerAuthHandler(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	h := bearerAuthHandler("secret", next)

	cases := []struct {
		name   string
		path   string
		header string
		want   int
	}{
		{"missing header", "/", "", http.StatusUnauthorized},
		{"wrong token", "/", "Bearer nope", http.StatusUnauthorized},
		{"wrong scheme", "/", "Basic secret", http.StatusUnauthorized},
		{"correct token", "/", "Bearer secret", http.StatusOK},
		{"mcp without token", "/mcp", "", http.StatusUnauthorized},
		{"healthz is open", "/healthz", "", http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, c.path, nil)
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Errorf("status = %d, want %d", rec.Code, c.want)
			}
			if c.want == http.StatusUnauthorized && rec.Header().Get("WWW-Authenticate") == "" {
				t.Error("401 without WWW-Authenticate header")
			}
		})
	}
}
