package index

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/piligrim/marvindocs/internal/chunk"
	"github.com/piligrim/marvindocs/internal/docstore"
	"github.com/piligrim/marvindocs/internal/embed"
)

// maxEmbedText caps the text sent to the embedder, in bytes. Chunks are
// at most ~2400 bytes, so this only trims the breadcrumb of odd pages.
const maxEmbedText = 4000

// EmbedStats counts what an EmbedDoc / EmbedAll run stored.
type EmbedStats struct{ Chunks, Pages int }

// pendingChunks counts chunks of slug that have no vector yet.
func (i *Index) pendingChunks(ctx context.Context, slug string) (int, error) {
	var n int
	err := i.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM chunks c JOIN pages p ON p.id = c.page_id
		 LEFT JOIN chunk_vecs v ON v.chunk_id = c.id
		 WHERE p.doc_slug = ? AND v.chunk_id IS NULL`, slug).Scan(&n)
	return n, err
}

// EmbedDoc embeds every chunk of slug that has no row in chunk_vecs yet
// and stores the quantised vectors, batch chunks per embedder call and
// per transaction, so an interrupted run keeps what it has done.
// progress, if not nil, is called after each batch with the number of
// chunks done and the number pending when the run started.
func (i *Index) EmbedDoc(ctx context.Context, s *docstore.Store, slug string, e embed.Embedder, batch int, progress func(done, total int)) (EmbedStats, error) {
	if batch <= 0 {
		batch = 64
	}
	var st EmbedStats
	total, err := i.pendingChunks(ctx, slug)
	if err != nil || total == 0 {
		return st, err
	}
	defer i.dropVecCache(slug)

	pages, err := i.pagesWithPending(ctx, slug)
	if err != nil {
		return st, err
	}
	docName := strings.SplitN(slug, "~", 2)[0]

	var ids []int64
	var texts []string
	flush := func() error {
		if len(ids) == 0 {
			return nil
		}
		if err := i.storeVectors(ctx, e, ids, texts); err != nil {
			return err
		}
		st.Chunks += len(ids)
		ids, texts = ids[:0], texts[:0]
		if progress != nil {
			progress(st.Chunks, total)
		}
		return nil
	}

	for _, p := range pages {
		if err := ctx.Err(); err != nil {
			return st, err
		}
		html, err := s.Page(slug, p.path)
		if err != nil {
			continue // stale index or missing db.json
		}
		md, err := docstore.ToMarkdown(html)
		if err != nil {
			continue
		}
		title := chunk.Title(md)
		rows, err := i.db.QueryContext(ctx,
			`SELECT id, off, len FROM chunks WHERE page_id = ? AND NOT EXISTS (SELECT 1 FROM chunk_vecs v WHERE v.chunk_id = chunks.id)`, p.id)
		if err != nil {
			return st, fmt.Errorf("list chunks %s/%s: %w", slug, p.path, err)
		}
		found := false
		for rows.Next() {
			var id int64
			var off, n int
			if err := rows.Scan(&id, &off, &n); err != nil {
				rows.Close()
				return st, err
			}
			if off < 0 || n < 0 || off+n > len(md) {
				continue
			}
			crumb := crumbOf(docName, title, chunk.HeadingAt(md, off))
			ids = append(ids, id)
			texts = append(texts, truncateUTF8(crumb+"\n"+md[off:off+n], maxEmbedText))
			found = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return st, err
		}
		if found {
			st.Pages++
		}
		if len(ids) >= batch {
			if err := flush(); err != nil {
				return st, err
			}
		}
	}
	return st, flush()
}

type pendingPage struct {
	id   int64
	path string
}

// pagesWithPending lists pages of slug that have a chunk without vector.
func (i *Index) pagesWithPending(ctx context.Context, slug string) ([]pendingPage, error) {
	rows, err := i.db.QueryContext(ctx,
		`SELECT p.id, p.path FROM pages p WHERE p.doc_slug = ? AND EXISTS (
			SELECT 1 FROM chunks c LEFT JOIN chunk_vecs v ON v.chunk_id = c.id
			WHERE c.page_id = p.id AND v.chunk_id IS NULL)
		 ORDER BY p.id`, slug)
	if err != nil {
		return nil, fmt.Errorf("list pages %s: %w", slug, err)
	}
	defer rows.Close()
	var out []pendingPage
	for rows.Next() {
		var p pendingPage
		if err := rows.Scan(&p.id, &p.path); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// storeVectors embeds texts and writes the vectors of chunk ids in one
// transaction.
func (i *Index) storeVectors(ctx context.Context, e embed.Embedder, ids []int64, texts []string) error {
	vecs, err := e.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	if len(vecs) != len(ids) {
		return fmt.Errorf("embed: got %d vectors for %d texts", len(vecs), len(ids))
	}
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO chunk_vecs(chunk_id, vec) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for n, v := range vecs {
		if _, err := stmt.ExecContext(ctx, ids[n], embed.Quantize(v)); err != nil {
			return fmt.Errorf("store vector: %w", err)
		}
	}
	return tx.Commit()
}

// EmbedAll runs EmbedDoc for every doc of the index (by slug), or only
// for docSlugs when non-empty. progress counts chunks over all docs.
func (i *Index) EmbedAll(ctx context.Context, s *docstore.Store, docSlugs []string, e embed.Embedder, batch int, progress func(done, total int)) (EmbedStats, error) {
	var st EmbedStats
	slugs := docSlugs
	if len(slugs) == 0 {
		var err error
		if slugs, err = i.docSlugs(ctx); err != nil {
			return st, err
		}
	}
	grand := 0
	for _, slug := range slugs {
		n, err := i.pendingChunks(ctx, slug)
		if err != nil {
			return st, err
		}
		grand += n
	}
	for _, slug := range slugs {
		base := st.Chunks
		var p func(done, total int)
		if progress != nil {
			p = func(done, _ int) { progress(base+done, grand) }
		}
		ds, err := i.EmbedDoc(ctx, s, slug, e, batch, p)
		st.Chunks += ds.Chunks
		st.Pages += ds.Pages
		if err != nil {
			return st, err
		}
	}
	return st, nil
}

// docSlugs lists every indexed doc slug in order.
func (i *Index) docSlugs(ctx context.Context) ([]string, error) {
	rows, err := i.db.QueryContext(ctx, `SELECT slug FROM docs ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// truncateUTF8 cuts s to at most n bytes without splitting a rune.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
