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
	Docs    int
	Entries int
	Pages   int
	// Touched counts docs whose mtime changed but content did not (Sync).
	Touched int
	// HashBackfilled counts docs whose missing content hash was computed (Sync).
	HashBackfilled int
	Duration       time.Duration
}

// Build (re)populates the index from the given docstore: symbols from
// index.json into entries and Markdown chunks of every page from
// db.json into chunks_fts. It replaces any existing rows; see Sync for
// the incremental variant.
func (i *Index) Build(ctx context.Context, s *docstore.Store) (BuildStats, error) {
	start := time.Now()
	var stats BuildStats

	if err := i.Reset(); err != nil {
		return stats, err
	}

	// Bulk-load PRAGMAs: trade durability for speed during the build.
	// If the process is killed mid-build the index will be corrupt, but
	// it is fully rebuildable. Restored after commit.
	restorePragmas := applyBulkPragmas(i.db)
	defer restorePragmas()

	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return stats, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	docStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO docs(slug, name, type, version, release, mtime, content_hash) VALUES (?, ?, ?, ?, ?, ?, ?)`)
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
		hash, err := s.ContentHash(d.Slug)
		if err != nil {
			return stats, fmt.Errorf("hash doc %s: %w", d.Slug, err)
		}
		if _, err := docStmt.ExecContext(ctx,
			d.Slug, d.Name, d.Type, d.Version, d.Release, d.Mtime, hash); err != nil {
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

		n, err := indexDocPages(ctx, s, d.Slug, tx)
		if err != nil {
			return stats, err
		}
		stats.Pages += n
		s.EvictDB(d.Slug)
	}

	if err := tx.Commit(); err != nil {
		return stats, fmt.Errorf("commit: %w", err)
	}

	// Post-build optimizations: FTS optimize + ANALYZE. Not fatal if they fail.
	_, _ = i.db.Exec(`INSERT INTO entries_fts(entries_fts) VALUES('optimize')`)
	_, _ = i.db.Exec(`INSERT INTO chunks_fts(chunks_fts) VALUES('optimize')`)
	_, _ = i.db.Exec(`ANALYZE`)

	stats.Duration = time.Since(start)
	return stats, nil
}

// Stats returns current counts from the index file.
type Stats struct {
	Docs    int
	Entries int
	Pages   int
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
	if err := i.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pages`).Scan(&st.Pages); err != nil {
		return st, err
	}
	return st, nil
}

// DocStat is the per-doc breakdown returned by PerDocStats.
type DocStat struct {
	Slug    string
	Entries int
	Pages   int
}

// PerDocStats returns one DocStat per indexed doc, sorted by entry
// count (descending). Useful for spotting outliers and verifying
// which docs ship with full-text bodies.
func (i *Index) PerDocStats(ctx context.Context) ([]DocStat, error) {
	rows, err := i.db.QueryContext(ctx,
		`SELECT slug FROM docs ORDER BY slug`)
	if err != nil {
		return nil, fmt.Errorf("list slugs: %w", err)
	}
	var out []DocStat
	for rows.Next() {
		var d DocStat
		if err := rows.Scan(&d.Slug); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	entRows, err := i.db.QueryContext(ctx,
		`SELECT doc_slug, COUNT(*) FROM entries GROUP BY doc_slug`)
	if err != nil {
		return nil, fmt.Errorf("entries per doc: %w", err)
	}
	entries := make(map[string]int)
	for entRows.Next() {
		var slug string
		var n int
		if err := entRows.Scan(&slug, &n); err != nil {
			entRows.Close()
			return nil, err
		}
		entries[slug] = n
	}
	entRows.Close()

	pageRows, err := i.db.QueryContext(ctx,
		`SELECT doc_slug, COUNT(*) FROM pages GROUP BY doc_slug`)
	pages := make(map[string]int)
	if err == nil {
		for pageRows.Next() {
			var slug string
			var n int
			if err := pageRows.Scan(&slug, &n); err != nil {
				pageRows.Close()
				return nil, err
			}
			pages[slug] = n
		}
		pageRows.Close()
	}

	for i := range out {
		out[i].Entries = entries[out[i].Slug]
		out[i].Pages = pages[out[i].Slug]
	}
	// sort descending by entries
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1].Entries < out[j].Entries; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out, nil
}
