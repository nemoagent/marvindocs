package docstore

import (
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

func TestPageDBLRUEviction(t *testing.T) {
	s, err := Open(dataRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	// Pick a few small docs that should always be present.
	s.dbCacheCap = 2
	for _, slug := range []string{"angular", "go", "react"} {
		if s.Doc(slug) == nil {
			t.Skipf("doc %s missing in dataset", slug)
		}
		if _, err := s.Page(slug, "index"); err != nil {
			// some docs use a different homepage path; touch any page
			idx, _ := s.Index(slug)
			if idx == nil || len(idx.Entries) == 0 {
				t.Skipf("doc %s has no entries", slug)
			}
			if _, err := s.Page(slug, idx.Entries[0].Path); err != nil {
				t.Fatalf("first page load for %s: %v", slug, err)
			}
		}
	}
	s.dbMu.Lock()
	defer s.dbMu.Unlock()
	if len(s.dbMem) > 2 {
		t.Fatalf("LRU should hold at most 2 entries, got %d (%v)", len(s.dbMem), s.dbLRU)
	}
	if len(s.dbLRU) != len(s.dbMem) {
		t.Fatalf("dbLRU and dbMem out of sync: lru=%v mem=%d", s.dbLRU, len(s.dbMem))
	}
}
