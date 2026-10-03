package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	h, err := Handler(Deps{})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/static/app.css", "/static/theme.js"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status %d", path, rec.Code)
		}
		want := map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"Referrer-Policy":        "no-referrer",
		}
		for k, v := range want {
			if got := rec.Header().Get(k); got != v {
				t.Errorf("GET %s: %s = %q, want %q", path, k, got, v)
			}
		}
		csp := rec.Header().Get("Content-Security-Policy")
		if csp == "" || !strings.Contains(csp, "script-src 'self';") {
			t.Errorf("GET %s: CSP missing or permits inline scripts: %q", path, csp)
		}
	}
}
