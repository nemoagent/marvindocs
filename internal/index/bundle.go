package index

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/piligrim/marvindocs/internal/docstore"
)

// A bundle index is a regular index file holding a subset of docs with
// their entries, pages, chunks and vectors, plus chunk_text: the
// breadcrumb and body of every chunk. chunks_fts is contentless and
// cannot be copied between files, so Merge fills it from chunk_text
// instead of converting the HTML pages again.
const chunkTextSQL = `CREATE TABLE IF NOT EXISTS chunk_text (
    chunk_id INTEGER PRIMARY KEY,
    crumb    TEXT NOT NULL,
    body     TEXT NOT NULL
)`

// Export writes the docs slugs of this index into a new bundle index at
// out (an existing file is replaced). Every doc must be indexed and
// unchanged in s since the last index build, so the re-cut chunks match
// the stored ones and the vectors stay valid.
func (i *Index) Export(ctx context.Context, s *docstore.Store, slugs []string, out string) error {
	if err := Remove(out); err != nil {
		return err
	}
	b, err := Open(out)
	if err != nil {
		return err
	}
	defer b.Close()
	if _, err := b.db.ExecContext(ctx, chunkTextSQL); err != nil {
		return err
	}
	conn, err := b.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS m`, "file:"+i.path+"?mode=ro"); err != nil {
		return fmt.Errorf("attach %s: %w", i.path, err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `DETACH DATABASE m`) }()

	for _, slug := range slugs {
		var hash string
		err := conn.QueryRowContext(ctx, `SELECT content_hash FROM m.docs WHERE slug = ?`, slug).Scan(&hash)
		if err == sql.ErrNoRows {
			return fmt.Errorf("doc %s is not indexed: run index build", slug)
		}
		if err != nil {
			return err
		}
		want, err := s.ContentHash(slug)
		if err != nil {
			return err
		}
		if hash != want {
			return fmt.Errorf("doc %s changed since the last index build: run index build", slug)
		}
		if err := exportDoc(ctx, conn, s, slug); err != nil {
			return fmt.Errorf("export %s: %w", slug, err)
		}
		s.EvictDB(slug)
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO entries_fts(entries_fts) VALUES('optimize')`)
	return err
}

// exportDoc copies the rows of one doc from m into the bundle with their
// ids and fills chunk_text from the re-converted pages.
func exportDoc(ctx context.Context, conn *sql.Conn, s *docstore.Store, slug string) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	copies := []string{
		`INSERT INTO docs(slug, name, type, version, release, mtime, content_hash)
		 SELECT slug, name, type, version, release, mtime, content_hash FROM m.docs WHERE slug = ?`,
		`INSERT INTO entries(id, doc_slug, name, path, type)
		 SELECT id, doc_slug, name, path, type FROM m.entries WHERE doc_slug = ?`,
		`INSERT INTO pages(id, doc_slug, path) SELECT id, doc_slug, path FROM m.pages WHERE doc_slug = ?`,
		`INSERT INTO chunks(id, page_id, off, len) SELECT c.id, c.page_id, c.off, c.len
		 FROM m.chunks c JOIN m.pages p ON p.id = c.page_id WHERE p.doc_slug = ?`,
		`INSERT INTO chunk_vecs(chunk_id, vec) SELECT v.chunk_id, v.vec FROM m.chunk_vecs v
		 JOIN m.chunks c ON c.id = v.chunk_id JOIN m.pages p ON p.id = c.page_id WHERE p.doc_slug = ?`,
	}
	for _, q := range copies {
		if _, err := tx.ExecContext(ctx, q, slug); err != nil {
			return err
		}
	}

	type stored struct{ id, off, len int }
	chunks := make(map[string][]stored) // page path -> chunks in offset order
	rows, err := tx.QueryContext(ctx,
		`SELECT p.path, c.id, c.off, c.len FROM pages p JOIN chunks c ON c.page_id = p.id
		 WHERE p.doc_slug = ? ORDER BY c.id`, slug)
	if err != nil {
		return err
	}
	for rows.Next() {
		var path string
		var c stored
		if err := rows.Scan(&path, &c.id, &c.off, &c.len); err != nil {
			rows.Close()
			return err
		}
		chunks[path] = append(chunks[path], c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	ins, err := tx.PrepareContext(ctx, `INSERT INTO chunk_text(chunk_id, crumb, body) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	docName := strings.SplitN(slug, "~", 2)[0]
	err = convertPages(ctx, s, slug, func(p pageMD) error {
		want, ok := chunks[p.path]
		if !ok {
			return fmt.Errorf("page %s is not indexed: run index build", p.path)
		}
		delete(chunks, p.path)
		if len(want) != len(p.chunks) {
			return fmt.Errorf("page %s: chunks differ from the index: run index build --full", p.path)
		}
		for k, c := range p.chunks {
			if c.Off != want[k].off || c.Len != want[k].len {
				return fmt.Errorf("page %s: chunks differ from the index: run index build --full", p.path)
			}
			if _, err := ins.ExecContext(ctx, want[k].id, crumbOf(docName, p.title, c.Heading), p.md[c.Off:c.Off+c.Len]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for path := range chunks {
		return fmt.Errorf("indexed page %s is missing from db.zip", path)
	}
	return tx.Commit()
}

// Merge adds every doc of the bundle index at path to this index,
// replacing docs with the same slug, and returns the merged slugs. Rows
// get fresh ids above the current ones; chunks_fts is filled from the
// bundle's chunk_text.
func (i *Index) Merge(ctx context.Context, path string) ([]string, error) {
	conn, err := i.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS b`, "file:"+path+"?mode=ro"); err != nil {
		return nil, fmt.Errorf("attach %s: %w", path, err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `DETACH DATABASE b`) }()

	var slugs []string
	rows, err := conn.QueryContext(ctx, `SELECT slug FROM b.docs ORDER BY slug`)
	if err != nil {
		return nil, fmt.Errorf("read bundle docs: %w", err)
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return nil, err
		}
		slugs = append(slugs, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var hasText int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM b.sqlite_master WHERE name = 'chunk_text'`).Scan(&hasText); err != nil || hasText == 0 {
		return nil, fmt.Errorf("%s is not a bundle index (no chunk_text)", path)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, s := range slugs {
		if err := deleteDocRows(ctx, tx, s); err != nil {
			return nil, err
		}
	}
	var pageOff, chunkOff int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM pages`).Scan(&pageOff); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM chunks`).Scan(&chunkOff); err != nil {
		return nil, err
	}
	steps := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO docs(slug, name, type, version, release, mtime, content_hash)
		  SELECT slug, name, type, version, release, mtime, content_hash FROM b.docs`, nil},
		{`INSERT INTO entries(doc_slug, name, path, type)
		  SELECT doc_slug, name, path, type FROM b.entries ORDER BY id`, nil},
		{`INSERT INTO pages(id, doc_slug, path) SELECT id + ?, doc_slug, path FROM b.pages`, []any{pageOff}},
		{`INSERT INTO chunks(id, page_id, off, len) SELECT id + ?, page_id + ?, off, len FROM b.chunks`,
			[]any{chunkOff, pageOff}},
		{`INSERT INTO chunk_vecs(chunk_id, vec) SELECT chunk_id + ?, vec FROM b.chunk_vecs`, []any{chunkOff}},
		{`INSERT INTO chunks_fts(rowid, crumb, body) SELECT chunk_id + ?, crumb, body FROM b.chunk_text`,
			[]any{chunkOff}},
	}
	for _, st := range steps {
		if _, err := tx.ExecContext(ctx, st.q, st.args...); err != nil {
			return nil, fmt.Errorf("merge %s: %w", path, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	i.vecMu.Lock()
	i.vecCache = nil
	i.vecMu.Unlock()
	_, _ = conn.ExecContext(ctx, `INSERT INTO entries_fts(entries_fts) VALUES('optimize')`)
	_, _ = conn.ExecContext(ctx, `INSERT INTO chunks_fts(chunks_fts) VALUES('optimize')`)
	_, _ = conn.ExecContext(ctx, `ANALYZE`)
	return slugs, nil
}
