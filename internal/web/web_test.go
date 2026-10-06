package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/piligrim/marvindocs/internal/docstore"
	"github.com/piligrim/marvindocs/internal/index"
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

// fixtureHandler wires a web handler to a one-doc index (slug "go",
// entry "Println") and a docstore whose docs.json lists the same doc.
// No db.zip is written, so only paths that stop before Store.Page work.
func fixtureHandler(t *testing.T) http.Handler {
	t.Helper()
	dir := t.TempDir()

	idx, err := index.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	for _, q := range []string{
		`INSERT INTO docs(slug, name, type, version, release, mtime) VALUES('go','GoFixture','lib','1','r1',0)`,
		`INSERT INTO entries(doc_slug, name, path, type) VALUES('go','Println','fmt/Println','Function')`,
		`INSERT INTO entries_fts(entries_fts) VALUES('rebuild')`,
	} {
		if _, err := idx.DB().Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	docsJSON := `[{"name":"GoFixture","slug":"go","type":"lib","version":"1","release":"r1"}]`
	if err := os.WriteFile(filepath.Join(dir, "docs.json"), []byte(docsJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := docstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	h, err := Handler(Deps{Store: store, Index: idx})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func get(h http.Handler, target string, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDocPathUnknown(t *testing.T) {
	h := fixtureHandler(t)
	for _, path := range []string{"/docs/", "/docs/nope/", "/docs/nope/some/page", "/other"} {
		if rec := get(h, path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status %d, want 404", path, rec.Code)
		}
	}
}

func TestIndexPage(t *testing.T) {
	rec := get(fixtureHandler(t), "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "GoFixture") {
		t.Errorf("body does not contain doc name: %s", rec.Body)
	}
}

func TestSearchPage(t *testing.T) {
	rec := get(fixtureHandler(t), "/search?q=Println")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Println") || !strings.Contains(body, "/docs/go/fmt/Println") {
		t.Errorf("body does not contain hit: %s", body)
	}
}

func TestDocPathETag(t *testing.T) {
	h := fixtureHandler(t)
	const etag = `"r1:fmt/Println"`
	rec := get(h, "/docs/go/fmt/Println", "If-None-Match", etag)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("status %d, want 304", rec.Code)
	}
	if got := rec.Header().Get("ETag"); got != etag {
		t.Errorf("ETag = %q, want %q", got, etag)
	}
	// Without a matching validator the handler proceeds to load the page;
	// the fixture has no db.zip, so a 404 proves it did not short-circuit.
	// A 404 must not carry cache validators.
	rec = get(h, "/docs/go/fmt/Println", "If-None-Match", `"stale"`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("stale ETag: status %d, want 404", rec.Code)
	}
	for _, k := range []string{"ETag", "Cache-Control"} {
		if v := rec.Header().Get(k); v != "" {
			t.Errorf("404 has %s=%q, want none", k, v)
		}
	}
}
