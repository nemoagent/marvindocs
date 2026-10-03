package index

import (
	"context"
	"fmt"
)

// compactThreshold is the share of free pages above which Compact runs
// VACUUM. Below it the file is left alone: VACUUM rewrites the whole
// database and is not worth it for a few percent.
const compactThreshold = 0.10

// Compact reclaims free pages left behind by deletes (incremental sync,
// --full rebuilds). It runs VACUUM only
// when more than compactThreshold of the file is free. Returns the
// number of bytes reclaimed, or 0 if nothing was done.
func (i *Index) Compact(ctx context.Context) (int64, error) {
	// Merge FTS5 segments first: deleted rows stay in dead segments until
	// a merge, and those pages never reach the freelist otherwise. A no-op
	// (cheap) when the tables were already optimized by the last build.
	for _, t := range []string{"entries_fts", "pages_fts"} {
		if _, err := i.db.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s(%s) VALUES('optimize')`, t, t)); err != nil {
			return 0, fmt.Errorf("optimize %s: %w", t, err)
		}
	}
	var pageCount, freeCount, pageSize int64
	if err := i.db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pageCount); err != nil {
		return 0, fmt.Errorf("page_count: %w", err)
	}
	if err := i.db.QueryRowContext(ctx, `PRAGMA freelist_count`).Scan(&freeCount); err != nil {
		return 0, fmt.Errorf("freelist_count: %w", err)
	}
	if err := i.db.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&pageSize); err != nil {
		return 0, fmt.Errorf("page_size: %w", err)
	}
	if pageCount == 0 || float64(freeCount)/float64(pageCount) <= compactThreshold {
		return 0, nil
	}
	if _, err := i.db.ExecContext(ctx, `VACUUM`); err != nil {
		return 0, fmt.Errorf("vacuum: %w", err)
	}
	// In WAL mode the rewritten database sits in the -wal file until a
	// checkpoint; without this the main file keeps its old size.
	if _, err := i.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return 0, fmt.Errorf("checkpoint: %w", err)
	}
	return freeCount * pageSize, nil
}
