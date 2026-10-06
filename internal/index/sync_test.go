package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/piligrim/marvindocs/internal/docstore"
)

func TestSyncIncrementalSkipsUnchangedDocs(t *testing.T) {
	root := dataRoot(t)
	store, err := docstore.Open(root)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "sync.db")
	idx, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	first, err := idx.Sync(context.Background(), store, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Docs == 0 || first.Entries == 0 {
		t.Fatalf("expected non-empty initial sync, got %+v", first)
	}

	// Second sync without any changes: should report the same totals,
	// taking far less wall time (no new inserts).
	second, err := idx.Sync(context.Background(), store, false)
	if err != nil {
		t.Fatal(err)
	}
	if second.Docs != first.Docs || second.Entries != first.Entries {
		t.Fatalf("expected stable counts, first=%+v second=%+v", first, second)
	}
	if second.Duration > first.Duration {
		t.Fatalf("expected faster no-op sync, first=%s second=%s", first.Duration, second.Duration)
	}
}

func TestSyncKeepsVectorsWhenOnlyMtimeChanges(t *testing.T) {
	idx, store := vecFixture(t)
	ctx := context.Background()
	root := store.Root()

	// Build stores the hash, so Sync has nothing to do.
	st, err := idx.Sync(ctx, store, false)
	if err != nil || st.HashBackfilled != 0 || st.Touched != 0 {
		t.Fatalf("no-op sync: %+v, %v", st, err)
	}
	// An index from before content_hash existed gets its hash backfilled.
	if _, err := idx.db.Exec(`UPDATE docs SET content_hash = ''`); err != nil {
		t.Fatal(err)
	}
	st, err = idx.Sync(ctx, store, false)
	if err != nil || st.HashBackfilled != 1 || st.Touched != 0 {
		t.Fatalf("backfill sync: %+v, %v", st, err)
	}
	if _, err := idx.EmbedAll(ctx, store, nil, &fakeEmbedder{}, 2, nil); err != nil {
		t.Fatal(err)
	}
	vecs := count(t, idx, "chunk_vecs")
	if vecs == 0 {
		t.Fatal("no vectors to keep")
	}

	reopen := func() *docstore.Store {
		s, err := docstore.Open(root)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	writeDocs := func(mtime string) {
		body := `[{"name":"Alpha","slug":"alpha","type":"lib","version":"1","release":"r1","mtime":` + mtime + `}]`
		if err := os.WriteFile(filepath.Join(root, "docs.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// New mtime, same db.zip/index.json: touched, vectors survive.
	writeDocs("2")
	st, err = idx.Sync(ctx, reopen(), false)
	if err != nil || st.Touched != 1 || st.HashBackfilled != 0 || st.Docs != 1 {
		t.Fatalf("touch sync: %+v, %v", st, err)
	}
	if n := count(t, idx, "chunk_vecs"); n != vecs {
		t.Fatalf("vectors after touch = %d, want %d", n, vecs)
	}
	var mtime int64
	if err := idx.DB().QueryRow(`SELECT mtime FROM docs WHERE slug = 'alpha'`).Scan(&mtime); err != nil || mtime != 2 {
		t.Fatalf("stored mtime = %d, %v", mtime, err)
	}

	// Forcing a rehash of an unchanged doc finds the same hash: nothing stored.
	st, err = idx.Sync(ctx, reopen(), true)
	if err != nil || st.HashBackfilled != 0 || st.Touched != 0 {
		t.Fatalf("rehash sync: %+v, %v", st, err)
	}

	// New mtime and changed db.zip: rebuilt, vectors dropped.
	writeDocs("3")
	writeDB(t, filepath.Join(root, "alpha"), map[string]string{
		"net/connect": "<h1>Connect</h1><p>Changed text.</p>",
	})
	st, err = idx.Sync(ctx, reopen(), false)
	if err != nil || st.Touched != 0 || st.Pages != 1 {
		t.Fatalf("rebuild sync: %+v, %v", st, err)
	}
	if n := count(t, idx, "chunk_vecs"); n != 0 {
		t.Fatalf("vectors after rebuild = %d, want 0", n)
	}
	if n, err := idx.VectorCount(ctx); err != nil || n != 0 {
		t.Fatalf("VectorCount = %d, %v", n, err)
	}
}
