package index

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/piligrim/marvindocs/internal/docstore"
)

// BuildOptions controls optional work done by Build.
type BuildOptions struct {
	// Fulltext, when true, also populates pages_fts with the plain-text
	// body of every page from db.json. This is slower and the resulting
	// index file grows to roughly the combined size of all dumps.
	Fulltext bool
}

// BuildStats is returned from Build to describe what was indexed.
type BuildStats struct {
	Docs     int
	Entries  int
	Pages    int // 0 unless Fulltext was requested
	Duration time.Duration
}

// Build (re)populates the index from the given docstore with default
// options (symbols only, no page bodies).
func (i *Index) Build(ctx context.Context, s *docstore.Store) (BuildStats, error) {
	return i.BuildWithOptions(ctx, s, BuildOptions{})
}

// BuildWithOptions (re)populates the index from the given docstore using
// the provided options. It replaces any existing rows — incremental
// updates are not supported yet.
func (i *Index) BuildWithOptions(ctx context.Context, s *docstore.Store, opts BuildOptions) (BuildStats, error) {
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

	var pageStmt *sql.Stmt
	if opts.Fulltext {
		pageStmt, err = tx.PrepareContext(ctx,
			`INSERT INTO pages_fts(doc_slug, path, body) VALUES (?, ?, ?)`)
		if err != nil {
			return stats, fmt.Errorf("prepare pages_fts: %w", err)
		}
		defer pageStmt.Close()
	}

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

		if opts.Fulltext {
			err := s.StreamPages(d.Slug, func(path, html string) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				body := docstore.ToPlainText(html)
				if body == "" {
					return nil
				}
				if _, err := pageStmt.ExecContext(ctx, d.Slug, path, body); err != nil {
					return fmt.Errorf("insert page %s/%s: %w", d.Slug, path, err)
				}
				stats.Pages++
				return nil
			})
			// db.json missing is non-fatal (some docs ship only index.json).
			if err != nil {
				return stats, err
			}
			s.EvictDB(d.Slug)
		}
	}

	if err := tx.Commit(); err != nil {
		return stats, fmt.Errorf("commit: %w", err)
	}

	// Post-build optimizations: FTS optimize + ANALYZE. Not fatal if they fail.
	_, _ = i.db.Exec(`INSERT INTO entries_fts(entries_fts) VALUES('optimize')`)
	if opts.Fulltext {
		_, _ = i.db.Exec(`INSERT INTO pages_fts(pages_fts) VALUES('optimize')`)
	}
	_, _ = i.db.Exec(`ANALYZE`)

	stats.Duration = time.Since(start)
	return stats, nil
}

// Stats returns current counts from the index file.
type Stats struct {
	Docs    int
	Entries int
	Pages   int // rows in pages_fts (0 if full-text indexing wasn't used)
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
	// pages_fts may not exist on an old index file — tolerate that.
	if err := i.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pages_fts`).Scan(&st.Pages); err != nil {
		st.Pages = 0
	}
	return st, nil
}
