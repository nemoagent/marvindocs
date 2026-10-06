package index

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/piligrim/marvindocs/internal/docstore"
)

// Sync incrementally updates the index to match the docstore. Docs whose
// mtime hasn't changed are left alone; docs whose mtime changed but whose
// content hash did not only get the new mtime, which keeps their chunk
// vectors; other new and modified docs are re-indexed; docs that
// disappeared from docstore are removed. A doc without page rows (index
// built by an older version without the chunk index) is re-indexed as
// well. Docs without a stored hash get one computed (HashBackfilled).
func (i *Index) Sync(ctx context.Context, s *docstore.Store) (BuildStats, error) {
	start := time.Now()
	var stats BuildStats

	existing, err := i.snapshotDocs(ctx)
	if err != nil {
		return stats, err
	}

	type plan struct {
		doc  docstore.Doc
		hash string // content hash to store with the rebuilt doc
	}
	var plans []plan
	hashes := make(map[string]string) // slug -> hash to store for kept docs
	wanted := make(map[string]struct{}, len(s.Docs()))
	for _, d := range s.Docs() {
		wanted[d.Slug] = struct{}{}
		ex, ok := existing[d.Slug]
		if ok && ex.hasPages && ex.mtime == d.Mtime && ex.hash != "" {
			stats.Docs++
			stats.Entries += ex.entries
			stats.Pages += ex.pages
			continue
		}
		hash, err := s.ContentHash(d.Slug)
		if err != nil {
			return stats, err
		}
		switch {
		case !ok || !ex.hasPages:
			plans = append(plans, plan{d, hash})
			continue
		case ex.mtime == d.Mtime: // old index without hashes
			hashes[d.Slug] = hash
			stats.HashBackfilled++
		case ex.hash == hash:
			hashes[d.Slug] = hash
			stats.Touched++
		default:
			plans = append(plans, plan{d, hash})
			continue
		}
		stats.Docs++
		stats.Entries += ex.entries
		stats.Pages += ex.pages
	}

	// Drop docs no longer present in the store.
	var toDelete []string
	for slug := range existing {
		if _, keep := wanted[slug]; !keep {
			toDelete = append(toDelete, slug)
		}
	}

	if len(plans) == 0 && len(toDelete) == 0 && len(hashes) == 0 {
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

	for slug, hash := range hashes {
		if _, err := tx.ExecContext(ctx, `UPDATE docs SET content_hash = ?, mtime = ? WHERE slug = ?`,
			hash, s.Doc(slug).Mtime, slug); err != nil {
			return stats, fmt.Errorf("update doc %s: %w", slug, err)
		}
	}

	docIns, err := tx.PrepareContext(ctx,
		`INSERT INTO docs(slug, name, type, version, release, mtime, content_hash) VALUES (?, ?, ?, ?, ?, ?, ?)`)
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

	for _, p := range plans {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		if err := deleteDocRows(ctx, tx, p.doc.Slug); err != nil {
			return stats, err
		}
		if _, err := docIns.ExecContext(ctx,
			p.doc.Slug, p.doc.Name, p.doc.Type, p.doc.Version, p.doc.Release, p.doc.Mtime, p.hash); err != nil {
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

		n, err := indexDocPages(ctx, s, p.doc.Slug, tx)
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
	_, _ = i.db.Exec(`INSERT INTO chunks_fts(chunks_fts) VALUES('optimize')`)
	_, _ = i.db.Exec(`ANALYZE`)

	stats.Duration = time.Since(start)
	return stats, nil
}

// docSnapshot is per-slug state read from the existing index.
type docSnapshot struct {
	mtime    int64
	hash     string
	entries  int
	pages    int
	hasPages bool
}

// snapshotDocs reads the current docs/entries/pages counts grouped by
// slug.
func (i *Index) snapshotDocs(ctx context.Context) (map[string]docSnapshot, error) {
	out := make(map[string]docSnapshot)

	rows, err := i.db.QueryContext(ctx, `SELECT slug, mtime, content_hash FROM docs`)
	if err != nil {
		return nil, fmt.Errorf("snapshot docs: %w", err)
	}
	for rows.Next() {
		var slug string
		var mtime int64
		var hash string
		if err := rows.Scan(&slug, &mtime, &hash); err != nil {
			rows.Close()
			return nil, err
		}
		out[slug] = docSnapshot{mtime: mtime, hash: hash}
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

	rows, err = i.db.QueryContext(ctx, `SELECT doc_slug, COUNT(*) FROM pages GROUP BY doc_slug`)
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
// Triggers keep entries_fts in sync; pages/chunks/chunk_vecs/chunks_fts
// are cleaned manually (the FTS rows by the rowids they share with chunks).
func deleteDocRows(ctx context.Context, tx *sql.Tx, slug string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM entries WHERE doc_slug = ?`, slug); err != nil {
		return fmt.Errorf("delete entries %s: %w", slug, err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM chunks_fts WHERE rowid IN (SELECT id FROM chunks WHERE page_id IN (SELECT id FROM pages WHERE doc_slug = ?))`, slug); err != nil {
		return fmt.Errorf("delete chunk index %s: %w", slug, err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM chunk_vecs WHERE chunk_id IN (SELECT id FROM chunks WHERE page_id IN (SELECT id FROM pages WHERE doc_slug = ?))`, slug); err != nil {
		return fmt.Errorf("delete chunk vectors %s: %w", slug, err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM chunks WHERE page_id IN (SELECT id FROM pages WHERE doc_slug = ?)`, slug); err != nil {
		return fmt.Errorf("delete chunks %s: %w", slug, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM pages WHERE doc_slug = ?`, slug); err != nil {
		return fmt.Errorf("delete pages %s: %w", slug, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM docs WHERE slug = ?`, slug); err != nil {
		return fmt.Errorf("delete doc %s: %w", slug, err)
	}
	return nil
}
