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
	if want := "# Connect Opens a [pooled] [connection] to the server."; hits[0].Snippet != want {
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

func TestOpenMigratesLegacyPagesFTS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	idx, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the page-level layout: pages_fts plus an indexed page row.
	for _, q := range []string{
		`CREATE VIRTUAL TABLE pages_fts USING fts5(body, content='', contentless_delete=1)`,
		`INSERT INTO pages(id, doc_slug, path) VALUES (1, 'a', 'x')`,
		`INSERT INTO pages_fts(rowid, body) VALUES (1, 'legacy text')`,
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
	var n int
	if err := idx.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'pages_fts'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("pages_fts survived: n=%d err=%v", n, err)
	}
	if err := idx.DB().QueryRow(`SELECT COUNT(*) FROM pages`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("page rows survived: n=%d err=%v", n, err)
	}
}

// chunkFixtureIndex opens an index holding one doc "go" whose page p
// has two chunks about "gopher" and one about "badger", and a page q
// with one chunk about "gopher".
func chunkFixtureIndex(t *testing.T) *Index {
	t.Helper()
	idx, err := Open(filepath.Join(t.TempDir(), "chunks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	stmts := []string{
		`INSERT INTO docs(slug, name, type, version, release, mtime) VALUES ('go', 'Go', 'lib', '1', 'r', 0)`,
		`INSERT INTO pages(id, doc_slug, path) VALUES (1, 'go', 'p'), (2, 'go', 'q')`,
		`INSERT INTO chunks(id, page_id, off, len) VALUES (1, 1, 0, 10), (2, 1, 10, 10), (3, 1, 20, 10), (4, 2, 0, 10)`,
		`INSERT INTO chunks_fts(rowid, crumb, body) VALUES
			(1, 'go > P', 'gopher gopher gopher digs'),
			(2, 'go > P > More', 'the gopher again'),
			(3, 'go > P > Other', 'badger only'),
			(4, 'go > Q', 'a lone gopher among many other words here')`,
	}
	for _, q := range stmts {
		if _, err := idx.DB().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return idx
}

func TestSearchPagesAggregatesChunks(t *testing.T) {
	idx := chunkFixtureIndex(t)
	hits, err := idx.SearchPages(context.Background(), nil, "gopher", "go", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].Path != "p" || hits[1].Path != "q" {
		t.Fatalf("hits = %+v, want one hit per page, p first", hits)
	}
}

func TestSearchChunksLimits(t *testing.T) {
	idx := chunkFixtureIndex(t)
	ctx := context.Background()

	hits, err := idx.SearchChunks(ctx, "gopher", "go", 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].Path != "p" || hits[0].Off != 0 || hits[1].Path != "q" {
		t.Fatalf("perPage=1: %+v", hits)
	}

	hits, err = idx.SearchChunks(ctx, "gopher", "go", 10, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 || hits[0].Path != "p" || hits[1].Path != "p" || hits[2].Path != "q" {
		t.Fatalf("perPage=5: %+v", hits)
	}

	hits, err = idx.SearchChunks(ctx, "gopher", "go", 1, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].Path != "p" || hits[1].Path != "p" {
		t.Fatalf("pages=1: %+v", hits)
	}
}

func TestBuildDeleteDocRemovesChunks(t *testing.T) {
	store := fixtureStore(t)
	idx, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	ctx := context.Background()
	if _, err := idx.Build(ctx, store); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := idx.DB().QueryRow(`SELECT COUNT(*) FROM chunks`).Scan(&n); err != nil || n == 0 {
		t.Fatalf("chunks after build: n=%d err=%v", n, err)
	}
	tx, err := idx.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := deleteDocRows(ctx, tx, "alpha"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := idx.DB().QueryRow(`SELECT COUNT(*) FROM chunks`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("chunks after delete: n=%d err=%v", n, err)
	}
	hits, err := idx.SearchPages(ctx, nil, "pooled", "", 10)
	if err != nil || len(hits) != 0 {
		t.Fatalf("hits after delete = %+v, %v", hits, err)
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
