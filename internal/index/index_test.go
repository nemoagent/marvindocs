package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// dataRoot points at the project's data/public/docs directory. Tests are
// skipped if the data isn't present or under -short: building the shared
// index from a full devdocs tree takes minutes, so the fast suite relies
// on the small fixture-based tests only.
func dataRoot(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping full-index test in -short mode")
	}
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

func TestOpenAndReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	idx, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	if err := idx.Reset(); err != nil {
		t.Fatal(err)
	}
	st, err := idx.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Docs != 0 || st.Entries != 0 {
		t.Fatalf("expected empty index, got %+v", st)
	}
}

func TestBuildPopulatesDocsAndEntries(t *testing.T) {
	idx := sharedIndex(t)

	got, err := idx.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Docs == 0 || got.Entries == 0 {
		t.Fatalf("expected non-empty stats, got %+v", got)
	}

	// Sanity: the angular doc should have been recorded.
	var name string
	row := idx.DB().QueryRow(`SELECT name FROM docs WHERE slug = 'angular'`)
	if err := row.Scan(&name); err != nil {
		t.Fatalf("angular not found in docs: %v", err)
	}
	if name == "" {
		t.Fatal("empty name for angular")
	}

	// Sanity: FTS should find at least one entry for a common term.
	var hits int
	row = idx.DB().QueryRow(`SELECT COUNT(*) FROM entries_fts WHERE entries_fts MATCH 'component'`)
	if err := row.Scan(&hits); err != nil {
		t.Fatal(err)
	}
	if hits == 0 {
		t.Fatal("expected FTS hits for 'component'")
	}
}
