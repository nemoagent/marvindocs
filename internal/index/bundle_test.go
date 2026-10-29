package index

import (
	"context"
	"path/filepath"
	"testing"
)

func TestExportMerge(t *testing.T) {
	ctx := context.Background()
	src, store := vecFixture(t)
	if _, err := src.EmbedAll(ctx, store, nil, &fakeEmbedder{}, 2, nil); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "bundle.db")
	if err := src.Export(ctx, store, []string{"alpha"}, out); err != nil {
		t.Fatal(err)
	}

	// The target already has alpha (replaced) and beta (kept).
	dst, err := Open(filepath.Join(t.TempDir(), "dst.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	if _, err := dst.Build(ctx, fixtureStore(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := dst.DB().Exec(`DROP TABLE IF EXISTS sqlite_stat1`); err != nil {
		t.Fatal(err)
	}
	slugs, err := dst.Merge(ctx, out)
	if err != nil {
		t.Fatal(err)
	}
	if n := count(t, dst, "sqlite_stat1"); n == 0 {
		t.Fatal("Merge left no planner stats (ANALYZE failed)")
	}
	if len(slugs) != 1 || slugs[0] != "alpha" {
		t.Fatalf("merged %v", slugs)
	}
	if n := count(t, dst, "docs"); n != 2 {
		t.Fatalf("docs = %d, want 2", n)
	}
	if n, want := count(t, dst, "chunk_vecs"), count(t, src, "chunk_vecs"); n != want || n == 0 {
		t.Fatalf("vectors = %d, want %d", n, want)
	}
	var hits int
	if err := dst.DB().QueryRow(`SELECT COUNT(*) FROM chunks_fts f JOIN chunks c ON c.id = f.rowid
		JOIN pages p ON p.id = c.page_id WHERE chunks_fts MATCH 'pooled' AND p.doc_slug = 'alpha'`).Scan(&hits); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("fts hits = %d, want 1", hits)
	}
	if n := count(t, dst, "chunks_fts"); n != count(t, dst, "chunks") {
		t.Fatalf("chunks_fts rows = %d, chunks = %d", n, count(t, dst, "chunks"))
	}
	if n := count(t, dst, "entries WHERE doc_slug = 'beta'"); n != 1 {
		t.Fatalf("beta entries = %d", n)
	}
}
