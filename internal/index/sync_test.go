package index

import (
	"context"
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

	first, err := idx.Sync(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	if first.Docs == 0 || first.Entries == 0 {
		t.Fatalf("expected non-empty initial sync, got %+v", first)
	}

	// Second sync without any changes: should report the same totals,
	// taking far less wall time (no new inserts).
	second, err := idx.Sync(context.Background(), store)
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
