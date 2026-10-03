package index

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	hits, err := idx.SearchPages(ctx, store, "pooled connection", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].DocSlug != "alpha" || hits[0].Path != "net/connect" {
		t.Fatalf("unexpected page hits: %+v", hits)
	}
	if want := "Connect Opens a [pooled] [connection] to the server."; hits[0].Snippet != want {
		t.Fatalf("snippet = %q, want %q", hits[0].Snippet, want)
	}

	// Without a page source hits come back without snippets.
	hits, err = idx.SearchPages(ctx, nil, "pooled", "alpha", 10)
	if err != nil || len(hits) != 1 || hits[0].Snippet != "" {
		t.Fatalf("hits without page source = %+v, %v", hits, err)
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
	if _, err := idx.DB().Exec(`DELETE FROM pages WHERE doc_slug = 'alpha'`); err != nil {
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

func TestOpenDropsLegacyPagesFTS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	idx, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Replace pages_fts with the pre-contentless layout that stored text.
	for _, q := range []string{
		`DROP TABLE pages_fts`,
		`CREATE VIRTUAL TABLE pages_fts USING fts5(doc_slug UNINDEXED, path UNINDEXED, body)`,
		`INSERT INTO pages_fts(doc_slug, path, body) VALUES ('a', 'x', 'legacy text')`,
	} {
		if _, err := idx.DB().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	_ = idx.Close()

	idx, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	var ddl string
	if err := idx.DB().QueryRow(`SELECT sql FROM sqlite_master WHERE name = 'pages_fts'`).Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ddl, "content=''") {
		t.Fatalf("pages_fts not migrated: %s", ddl)
	}
	var n int
	if err := idx.DB().QueryRow(`SELECT COUNT(*) FROM pages_fts WHERE pages_fts MATCH 'legacy'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("legacy rows survived: n=%d err=%v", n, err)
	}
}

func TestSnippet(t *testing.T) {
	words := make([]string, 60)
	for i := range words {
		words[i] = "w" + strconv.Itoa(i)
	}
	words[30] = "Pooled"
	text := strings.Join(words, " ")

	got := snippet(text, "pool")
	if !strings.HasPrefix(got, "… w24 ") || !strings.Contains(got, "[Pooled]") || !strings.HasSuffix(got, " w47 …") {
		t.Fatalf("snippet = %q", got)
	}
	if got := snippet("short text only", "zzz"); got != "short text only" {
		t.Fatalf("no-match snippet = %q", got)
	}
}
