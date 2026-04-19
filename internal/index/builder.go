package index

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/piligrim/marvindocs/internal/docstore"
)

// BuildStats is returned from Build to describe what was indexed.
type BuildStats struct {
	Docs     int
	Entries  int
	Duration time.Duration
}

// Build (re)populates the index from the given docstore. It replaces any
// existing rows — incremental updates are not supported yet.
func (i *Index) Build(ctx context.Context, s *docstore.Store) (BuildStats, error) {
	start := time.Now()
	var stats BuildStats

	if err := i.Reset(); err != nil {
		return stats, err
	}

	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return stats, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	docStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO docs(slug, name, type, version, release, mtime) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return stats, fmt.Errorf("prepare docs: %w", err)
	}
	defer docStmt.Close()

	entStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO entries(doc_slug, name, path, type) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return stats, fmt.Errorf("prepare entries: %w", err)
	}
	defer entStmt.Close()

	for _, d := range s.Docs() {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		if _, err := docStmt.ExecContext(ctx,
			d.Slug, d.Name, d.Type, d.Version, d.Release, d.Mtime); err != nil {
			return stats, fmt.Errorf("insert doc %s: %w", d.Slug, err)
		}
		stats.Docs++

		idx, err := s.Index(d.Slug)
		if err != nil {
			// Missing index.json is not fatal — skip the doc's entries.
			continue
		}
		for _, e := range idx.Entries {
			if _, err := entStmt.ExecContext(ctx, d.Slug, e.Name, e.Path, e.Type); err != nil {
				return stats, fmt.Errorf("insert entry %s/%s: %w", d.Slug, e.Path, err)
			}
			stats.Entries++
		}
	}

	if err := tx.Commit(); err != nil {
		return stats, fmt.Errorf("commit: %w", err)
	}

	// Post-build optimizations: FTS optimize + ANALYZE. Not fatal if they fail.
	_, _ = i.db.Exec(`INSERT INTO entries_fts(entries_fts) VALUES('optimize')`)
	_, _ = i.db.Exec(`ANALYZE`)

	stats.Duration = time.Since(start)
	return stats, nil
}

// Stats returns current counts from the index file.
type Stats struct {
	Docs    int
	Entries int
}

// Stats queries the index for summary counts.
func (i *Index) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	row := i.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM docs`)
	if err := row.Scan(&st.Docs); err != nil {
		if err == sql.ErrNoRows {
			return st, nil
		}
		return st, err
	}
	row = i.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM entries`)
	if err := row.Scan(&st.Entries); err != nil {
		return st, err
	}
	return st, nil
}
