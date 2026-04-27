package index

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// SearchHit is one result from Search.
type SearchHit struct {
	DocSlug string  `json:"doc_slug"`
	Name    string  `json:"name"`
	Path    string  `json:"path"`
	Type    string  `json:"type"`
	Score   float64 `json:"score"` // lower = better (bm25)
}

// Search queries the FTS index for entries whose name/path match the query.
// If docSlug is non-empty, results are restricted to that doc.
// Limit must be > 0.
//
// Ranking notes:
//   - bm25(entries_fts, 2.0, 1.0) gives the entry name twice the weight
//     of the path. Name matches are usually what users want
//     (e.g. searching "useState" should rank the function above pages
//     that merely mention "useState" in their URL path).
//   - Without a doc filter, BM25 alone tends to surface short paths from
//     exotic docs (Pony, Nim) above mainstream ones. We bias the score
//     by a small constant for "popular" docs so that, all else equal,
//     a hit in postgresql/python/react/go beats one in haxe/pony.
func (i *Index) Search(ctx context.Context, query, docSlug string, limit int) ([]SearchHit, error) {
	q := sanitizeFTSQuery(query)
	if q == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}

	// Over-fetch when ranking globally so the popularity boost has
	// candidates to reorder. With a doc filter, no boost is applied
	// and the FTS LIMIT is enough.
	fetchLimit := limit
	if docSlug == "" {
		fetchLimit = limit * 4
		if fetchLimit < 50 {
			fetchLimit = 50
		}
	}

	var (
		rows interface {
			Next() bool
			Scan(dest ...any) error
			Close() error
			Err() error
		}
		err error
	)

	if docSlug == "" {
		rows, err = i.db.QueryContext(ctx, `
			SELECT e.doc_slug, e.name, e.path, e.type, bm25(entries_fts, 2.0, 1.0) AS score
			FROM entries_fts
			JOIN entries e ON e.id = entries_fts.rowid
			WHERE entries_fts MATCH ?
			ORDER BY score
			LIMIT ?`, q, fetchLimit)
	} else {
		rows, err = i.db.QueryContext(ctx, `
			SELECT e.doc_slug, e.name, e.path, e.type, bm25(entries_fts, 2.0, 1.0) AS score
			FROM entries_fts
			JOIN entries e ON e.id = entries_fts.rowid
			WHERE entries_fts MATCH ? AND e.doc_slug = ?
			ORDER BY score
			LIMIT ?`, q, docSlug, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.DocSlug, &h.Name, &h.Path, &h.Type, &h.Score); err != nil {
			return nil, err
		}
		if docSlug == "" {
			h.Score -= popularityBoost(h.DocSlug)
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if docSlug == "" {
		// Re-sort and trim after applying the boost.
		sortHitsByScore(hits)
		if len(hits) > limit {
			hits = hits[:limit]
		}
	}

	// Augment with substring matches on entries.name only when the
	// FTS prefix-tokenizer didn't already saturate the result set.
	// Calling LIKE on every search adds ~30 ms on cold queries; we
	// skip it when FTS already returned at least limit/2 hits.
	if len(hits) < limit/2 || len(hits) == 0 {
		fb, err := i.searchSubstring(ctx, query, docSlug, limit)
		if err != nil {
			return nil, err
		}
		hits = mergeHits(hits, fb, limit)
	}
	return hits, nil
}

// mergeHits concatenates FTS and substring hits, dedupes by
// (doc_slug, path), and trims to limit. FTS results are kept first
// because they carry a real bm25 score; substring fallbacks fill the
// remaining slots.
func mergeHits(primary, fallback []SearchHit, limit int) []SearchHit {
	seen := make(map[string]struct{}, len(primary)+len(fallback))
	out := make([]SearchHit, 0, limit)
	for _, h := range primary {
		key := h.DocSlug + "\x00" + h.Path
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, h)
		if len(out) >= limit {
			return out
		}
	}
	for _, h := range fallback {
		key := h.DocSlug + "\x00" + h.Path
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, h)
		if len(out) >= limit {
			return out
		}
	}
	return out
}

// searchSubstring is the LIKE-based fallback used to augment FTS
// prefix-match hits. For each whitespace-separated token in the query,
// it looks for the token as a case-insensitive substring inside
// entries.name. Multi-token queries are AND'd together, so
// 'context cancel' matches names that contain both substrings (e.g.
// 'context.WithCancel'). The synthetic score (length-based, always
// positive) sorts these below real FTS hits whose bm25 is negative.
func (i *Index) searchSubstring(ctx context.Context, query, docSlug string, limit int) ([]SearchHit, error) {
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return nil, nil
	}

	var clauses []string
	args := make([]any, 0, len(tokens)+2)
	for _, t := range tokens {
		clauses = append(clauses, `name LIKE ? ESCAPE '\'`)
		args = append(args, "%"+escapeLike(t)+"%")
	}
	where := strings.Join(clauses, " AND ")
	if docSlug != "" {
		where += " AND doc_slug = ?"
		args = append(args, docSlug)
	}
	args = append(args, limit*4)

	rows, err := i.db.QueryContext(ctx,
		`SELECT doc_slug, name, path, type FROM entries WHERE `+where+` LIMIT ?`,
		args...)
	if err != nil {
		return nil, fmt.Errorf("search substring: %w", err)
	}
	defer rows.Close()

	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.DocSlug, &h.Name, &h.Path, &h.Type); err != nil {
			return nil, err
		}
		// Synthetic positive score: shorter names beat longer ones.
		// Always greater than any real (negative) bm25 score, so
		// FTS hits keep precedence after merge.
		h.Score = float64(len(h.Name))
		if docSlug == "" {
			h.Score -= popularityBoost(h.DocSlug)
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sortHitsByScore(hits)
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

// escapeLike protects the user query from LIKE metacharacters so a
// literal underscore or percent doesn't act as a wildcard.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// sortHitsByScore sorts hits ascending (lower bm25 = better).
func sortHitsByScore(hits []SearchHit) {
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j-1].Score > hits[j].Score; j-- {
			hits[j-1], hits[j] = hits[j], hits[j-1]
		}
	}
}

// SearchPages queries pages_fts for pages whose body text matches the
// query. Requires that the index was built with Fulltext=true; otherwise
// it returns an empty slice. Snippet is a short excerpt around the match
// (~20 tokens, with HTML-ish highlight markers). docSlug restricts the
// search to a single doc when non-empty.
func (i *Index) SearchPages(ctx context.Context, query, docSlug string, limit int) ([]PageHit, error) {
	q := sanitizeFTSQuery(query)
	if q == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}

	var (
		rows *sql.Rows
		err  error
	)
	if docSlug == "" {
		rows, err = i.db.QueryContext(ctx, `
			SELECT doc_slug, path,
			       snippet(pages_fts, 2, '[', ']', ' … ', 16) AS snip,
			       bm25(pages_fts) AS score
			FROM pages_fts
			WHERE pages_fts MATCH ?
			ORDER BY score
			LIMIT ?`, q, limit)
	} else {
		rows, err = i.db.QueryContext(ctx, `
			SELECT doc_slug, path,
			       snippet(pages_fts, 2, '[', ']', ' … ', 16) AS snip,
			       bm25(pages_fts) AS score
			FROM pages_fts
			WHERE pages_fts MATCH ? AND doc_slug = ?
			ORDER BY score
			LIMIT ?`, q, docSlug, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("search pages: %w", err)
	}
	defer rows.Close()

	var hits []PageHit
	for rows.Next() {
		var h PageHit
		if err := rows.Scan(&h.DocSlug, &h.Path, &h.Snippet, &h.Score); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// PageHit is one result from SearchPages.
type PageHit struct {
	DocSlug string  `json:"doc_slug"`
	Path    string  `json:"path"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

// ListDocs returns all indexed documents ordered by slug.
func (i *Index) ListDocs(ctx context.Context) ([]DocRow, error) {
	rows, err := i.db.QueryContext(ctx,
		`SELECT slug, name, type, version, release, mtime FROM docs ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DocRow
	for rows.Next() {
		var d DocRow
		if err := rows.Scan(&d.Slug, &d.Name, &d.Type, &d.Version, &d.Release, &d.Mtime); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DocRow mirrors a row from the docs table.
type DocRow struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Version string `json:"version"`
	Release string `json:"release"`
	Mtime   int64  `json:"mtime"`
}

// sanitizeFTSQuery escapes a user-supplied query so that arbitrary input
// can't produce FTS5 syntax errors. Each whitespace-separated token is
// wrapped in double quotes with embedded quotes doubled; a trailing * is
// appended to enable prefix matching. An empty input yields an empty
// string (caller should treat as "no query").
func sanitizeFTSQuery(q string) string {
	fields := strings.Fields(q)
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		// drop tokens that become empty after quote-escaping
		escaped := strings.ReplaceAll(f, `"`, `""`)
		if escaped == "" {
			continue
		}
		parts = append(parts, `"`+escaped+`"*`)
	}
	return strings.Join(parts, " ")
}
