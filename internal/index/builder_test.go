package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/piligrim/marvindocs/internal/docstore"
)

// fixtureStore writes a tiny devdocs tree: one doc with index.json and
// db.json, and a second doc that ships only index.json.
func fixtureStore(t *testing.T) *docstore.Store {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"docs.json": `[{"name":"Alpha","slug":"alpha","type":"lib","version":"1","release":"r1","mtime":1},
			{"name":"Beta","slug":"beta","type":"lib","version":"1","release":"r1","mtime":1}]`,
		"alpha/index.json": `{"entries":[{"name":"Connect","path":"net/connect","type":"Function"}],"types":[]}`,
		"alpha/db.json":    `{"net/connect":"<h1>Connect</h1><p>Opens a pooled connection to the server.</p>"}`,
		"beta/index.json":  `{"entries":[{"name":"Other","path":"other","type":"Function"}],"types":[]}`,
	}
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := docstore.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestBuildIndexesPageBodies(t *testing.T) {
	store := fixtureStore(t)
	idx, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	ctx := context.Background()
	stats, err := idx.Build(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Docs != 2 || stats.Entries != 2 || stats.Pages != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	hits, err := idx.SearchPages(ctx, "pooled connection", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].DocSlug != "alpha" || hits[0].Path != "net/connect" {
		t.Fatalf("unexpected page hits: %+v", hits)
	}
}

func TestSyncIndexesPageBodies(t *testing.T) {
	store := fixtureStore(t)
	idx, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	ctx := context.Background()
	stats, err := idx.Sync(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Docs != 2 || stats.Pages != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	// Simulate an index built by an older version without page bodies:
	// the next sync must re-index the doc even though its mtime is the same.
	if _, err := idx.DB().Exec(`DELETE FROM pages_fts WHERE doc_slug = 'alpha'`); err != nil {
		t.Fatal(err)
	}
	stats, err = idx.Sync(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Pages != 1 {
		t.Fatalf("expected pages to be re-indexed, got %+v", stats)
	}
}
