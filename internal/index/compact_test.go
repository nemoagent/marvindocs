package index

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "idx.db")
	idx, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	ctx := context.Background()

	// Fresh file: nothing to reclaim.
	if n, err := idx.Compact(ctx); err != nil || n != 0 {
		t.Fatalf("fresh Compact = %d, %v; want 0, nil", n, err)
	}

	// Fill chunks_fts with enough data to span many pages, then delete it
	// all so the freelist dominates the file.
	body := strings.Repeat("lorem ipsum dolor sit amet ", 200)
	for i := 0; i < 500; i++ {
		if _, err := idx.db.Exec(`INSERT INTO chunks_fts(rowid, crumb, body) VALUES (?, ?, ?)`, i+1, "crumb", body); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := idx.db.Exec(`DELETE FROM chunks_fts`); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)

	n, err := idx.Compact(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("Compact reclaimed nothing after deleting all pages")
	}
	after, _ := os.Stat(path)
	if after.Size() >= before.Size() {
		t.Fatalf("file did not shrink: %d -> %d", before.Size(), after.Size())
	}
	// Second pass is a no-op.
	if n, err := idx.Compact(ctx); err != nil || n != 0 {
		t.Fatalf("second Compact = %d, %v; want 0, nil", n, err)
	}
}
