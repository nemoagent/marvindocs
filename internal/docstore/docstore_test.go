package docstore

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dataRoot points at the project's data/public/docs directory. Tests are
// skipped if the data isn't present (e.g. in CI without the dump).
func dataRoot(t *testing.T) string {
	t.Helper()
	// tests run from package dir; walk up to repo root.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(wd, "..", "..", "data", "public", "docs")
	if _, err := os.Stat(filepath.Join(root, "docs.json")); err != nil {
		t.Skipf("devdocs data not available at %s", root)
	}
	return root
}

func TestOpenAndList(t *testing.T) {
	s, err := Open(dataRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Docs()) == 0 {
		t.Fatal("expected at least one doc")
	}
	if s.Doc("angular") == nil {
		t.Fatal("expected angular doc")
	}
}

func TestIndexAndPage(t *testing.T) {
	s, err := Open(dataRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	idx, err := s.Index("angular")
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Entries) == 0 || len(idx.Types) == 0 {
		t.Fatal("angular index should have entries and types")
	}
	html, err := s.Page("angular", "index")
	if err != nil {
		t.Fatal(err)
	}
	if len(html) < 100 {
		t.Fatalf("expected non-trivial page html, got %d bytes", len(html))
	}
}

func TestStreamPages(t *testing.T) {
	s, err := Open(dataRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	var count int
	var sample string
	if err := s.StreamPages("angular", func(path, html string) error {
		count++
		if path == "index" {
			sample = html
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("expected at least one streamed page")
	}
	if len(sample) < 100 {
		t.Fatalf("index page should be non-trivial, got %d bytes", len(sample))
	}
}

func TestPageSpansMatchFullDecode(t *testing.T) {
	s, err := Open(dataRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	slug := "go"
	if s.Doc(slug) == nil {
		t.Skipf("doc %s missing in dataset", slug)
	}
	f, err := os.Open(filepath.Join(s.root, slug, "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var full map[string]string
	if err := json.NewDecoder(f).Decode(&full); err != nil {
		t.Fatal(err)
	}
	n := 0
	for path, want := range full {
		got, err := s.Page(slug, path)
		if err != nil {
			t.Fatalf("Page(%s, %s): %v", slug, path, err)
		}
		if got != want {
			t.Fatalf("Page(%s, %s) differs from full decode", slug, path)
		}
		if n++; n == 200 {
			break
		}
	}
	if _, err := s.Page(slug, "no/such/page"); err == nil {
		t.Fatal("expected error for missing page")
	}
}

func TestScanSpans(t *testing.T) {
	pages := map[string]string{
		"index":      "<p>plain</p>",
		"quote":      `say "hi" and \\ back`,
		"ends/slash": `trailing \\`,
		"unicode/é":  "café — 😀",
		"empty":      "",
		"big":        strings.Repeat("x\"y\\", 3000), // spans several small chunks
	}
	raw, err := json.Marshal(pages)
	if err != nil {
		t.Fatal(err)
	}
	// Tiny buffer forces ErrBufferFull paths and chunk-boundary escapes.
	spans, err := scanSpans(bufio.NewReaderSize(bytes.NewReader(raw), 16))
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != len(pages) {
		t.Fatalf("got %d spans, want %d", len(spans), len(pages))
	}
	for path, want := range pages {
		sp, ok := spans[path]
		if !ok {
			t.Fatalf("missing span for %q", path)
		}
		var got string
		if err := json.Unmarshal(raw[sp.off:sp.off+sp.n], &got); err != nil {
			t.Fatalf("decode %q: %v", path, err)
		}
		if got != want {
			t.Fatalf("page %q = %q, want %q", path, got, want)
		}
	}
	for _, bad := range []string{`[]`, `{"a": 1}`, `{"a": "b" "c": "d"}`, `{"a": "b"`} {
		if _, err := scanSpans(bufio.NewReader(strings.NewReader(bad))); err == nil {
			t.Errorf("scanSpans(%s) should fail", bad)
		}
	}
}
