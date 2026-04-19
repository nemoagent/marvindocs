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
