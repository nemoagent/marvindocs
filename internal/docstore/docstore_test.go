package docstore

import (
	"errors"
	"os"
	"path/filepath"
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

func TestPageMatchesStreamPages(t *testing.T) {
	s, err := Open(dataRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	slug := "go"
	if s.Doc(slug) == nil {
		t.Skipf("doc %s missing in dataset", slug)
	}
	const limit = 200
	full := map[string]string{}
	stop := errors.New("enough")
	err = s.StreamPages(slug, func(path, html string) error {
		full[path] = html
		if len(full) == limit {
			return stop
		}
		return nil
	})
	if err != nil && !errors.Is(err, stop) {
		t.Fatal(err)
	}
	for path, want := range full {
		got, err := s.Page(slug, path)
		if err != nil {
			t.Fatalf("Page(%s, %s): %v", slug, path, err)
		}
		if got != want {
			t.Fatalf("Page(%s, %s) differs from StreamPages", slug, path)
		}
	}
	if _, err := s.Page(slug, "no/such/page"); err == nil {
		t.Fatal("expected error for missing page")
	}
}
