package index

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/piligrim/marvindocs/internal/docstore"
)

// Sync incrementally updates the index to match the docstore. Docs whose
// mtime hasn't changed are left alone; new and modified docs are
// re-indexed; docs that disappeared from docstore are removed. A doc
// whose page bodies are missing from pages_fts (index built by an older
// version without full-text) is re-indexed as well.
func (i *Index) Sync(ctx context.Context, s *docstore.Store) (BuildStats, error) {
	start := time.Now()
	var stats BuildStats

	existing, err := i.snapshotDocs(ctx)
	if err != nil {
		return stats, err
	}

	type plan struct {
		doc     docstore.Doc
		rebuild bool
	}
	var plans []plan
	wanted := make(map[string]struct{}, len(s.Docs()))
	for _, d := range s.Docs() {
		wanted[d.Slug] = struct{}{}
		ex, ok := existing[d.Slug]
		switch {
		case !ok:
			plans = append(plans, plan{d, true})
		case ex.mtime != d.Mtime:
			plans = append(plans, plan{d, true})
		case !ex.hasPages:
			plans = append(plans, plan{d, true})
		default:
			stats.Docs++
			stats.Entries += ex.entries
			stats.Pages += ex.pages
		}
	}

	// Drop docs no longer present in the store.
	var toDelete []string
	for slug := range existing {
		if _, keep := wanted[slug]; !keep {
			toDelete = append(toDelete, slug)
		}
	}

	if len(plans) == 0 && len(toDelete) == 0 {
		stats.Duration = time.Since(start)
		return stats, nil
	}

	restorePragmas := applyBulkPragmas(i.db)
	defer restorePragmas()

	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return stats, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, slug := range toDelete {
		if err := deleteDocRows(ctx, tx, slug); err != nil {
			return stats, err
		}
	}

	docIns, err := tx.PrepareContext(ctx,
		`INSERT INTO docs(slug, name, type, version, release, mtime) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return stats, fmt.Errorf("prepare docs: %w", err)
	}
	defer docIns.Close()

	entIns, err := tx.PrepareContext(ctx,
		`INSERT INTO entries(doc_slug, name, path, type) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return stats, fmt.Errorf("prepare entries: %w", err)
	}
	defer entIns.Close()

	pageIns, err := tx.PrepareContext(ctx,
		`INSERT INTO pages_fts(doc_slug, path, body) VALUES (?, ?, ?)`)
	if err != nil {
		return stats, fmt.Errorf("prepare pages_fts: %w", err)
	}
	defer pageIns.Close()

	for _, p := range plans {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		if err := deleteDocRows(ctx, tx, p.doc.Slug); err != nil {
			return stats, err
		}
		if _, err := docIns.ExecContext(ctx,
			p.doc.Slug, p.doc.Name, p.doc.Type, p.doc.Version, p.doc.Release, p.doc.Mtime); err != nil {
			return stats, fmt.Errorf("insert doc %s: %w", p.doc.Slug, err)
		}
		stats.Docs++

		idx, err := s.Index(p.doc.Slug)
		if err != nil {
			continue // missing index.json is non-fatal
		}
		for _, e := range idx.Entries {
			if _, err := entIns.ExecContext(ctx, p.doc.Slug, e.Name, e.Path, e.Type); err != nil {
				return stats, fmt.Errorf("insert entry %s/%s: %w", p.doc.Slug, e.Path, err)
			}
			stats.Entries++
		}

		n, err := indexDocPages(ctx, s, p.doc.Slug, pageIns)
		if err != nil {
			return stats, err
		}
		stats.Pages += n
		s.EvictDB(p.doc.Slug)
	}

	if err := tx.Commit(); err != nil {
		return stats, fmt.Errorf("commit: %w", err)
	}

	_, _ = i.db.Exec(`INSERT INTO entries_fts(entries_fts) VALUES('optimize')`)
	_, _ = i.db.Exec(`INSERT INTO pages_fts(pages_fts) VALUES('optimize')`)
	_, _ = i.db.Exec(`ANALYZE`)

	stats.Duration = time.Since(start)
	return stats, nil
}

// docSnapshot is per-slug state read from the existing index.
type docSnapshot struct {
	mtime    int64
	entries  int
	pages    int
	hasPages bool
}

// snapshotDocs reads the current docs/entries/pages_fts counts grouped by
// slug.
func (i *Index) snapshotDocs(ctx context.Context) (map[string]docSnapshot, error) {
	out := make(map[string]docSnapshot)

	rows, err := i.db.QueryContext(ctx, `SELECT slug, mtime FROM docs`)
	if err != nil {
		return nil, fmt.Errorf("snapshot docs: %w", err)
	}
	for rows.Next() {
		var slug string
		var mtime int64
		if err := rows.Scan(&slug, &mtime); err != nil {
			rows.Close()
			return nil, err
		}
		out[slug] = docSnapshot{mtime: mtime}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = i.db.QueryContext(ctx, `SELECT doc_slug, COUNT(*) FROM entries GROUP BY doc_slug`)
	if err != nil {
		return nil, fmt.Errorf("snapshot entries: %w", err)
	}
	for rows.Next() {
		var slug string
		var n int
		if err := rows.Scan(&slug, &n); err != nil {
			rows.Close()
			return nil, err
		}
		s := out[slug]
		s.entries = n
		out[slug] = s
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = i.db.QueryContext(ctx, `SELECT doc_slug, COUNT(*) FROM pages_fts GROUP BY doc_slug`)
	if err != nil {
		return nil, fmt.Errorf("snapshot pages: %w", err)
	}
	for rows.Next() {
		var slug string
		var n int
		if err := rows.Scan(&slug, &n); err != nil {
			rows.Close()
			return nil, err
		}
		s := out[slug]
		s.pages = n
		s.hasPages = n > 0
		out[slug] = s
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

// deleteDocRows removes a doc and all its dependent rows from the index.
// Triggers keep entries_fts in sync; pages_fts is contentless and must
// be cleaned manually.
func deleteDocRows(ctx context.Context, tx *sql.Tx, slug string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM entries WHERE doc_slug = ?`, slug); err != nil {
		return fmt.Errorf("delete entries %s: %w", slug, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM pages_fts WHERE doc_slug = ?`, slug); err != nil {
		return fmt.Errorf("delete pages %s: %w", slug, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM docs WHERE slug = ?`, slug); err != nil {
		return fmt.Errorf("delete doc %s: %w", slug, err)
	}
	return nil
}
