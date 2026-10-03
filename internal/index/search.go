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

// docCatalog loads all doc slugs once and caches them.
func (i *Index) docCatalog(ctx context.Context) (slugs map[string]bool, names map[string]string, err error) {
	i.catalogOnce.Do(func() {
		docs, e := i.ListDocs(ctx)
		if e != nil {
			i.catalogErr = e
			return
		}
		i.catalogSlugs = make(map[string]bool, len(docs))
		i.catalogNames = make(map[string]string, len(docs))
		for _, d := range docs {
			i.catalogSlugs[d.Slug] = true
			norm := strings.ToLower(d.Slug)
			if idx := strings.Index(norm, "~"); idx >= 0 {
				prefix := norm[:idx]
				i.catalogNames[prefix] = prefix
				if dash := strings.LastIndex(prefix, "-"); dash >= 0 {
					base := prefix[:dash]
					if base != "" {
						if _, exists := i.catalogNames[base]; !exists {
							i.catalogNames[base] = base
						}
					}
				}
			}
		}
	})
	return i.catalogSlugs, i.catalogNames, i.catalogErr
}

// docAliases maps short tokens to canonical slug prefixes.
var docAliases = map[string]string{
	"js":    "javascript",
	"py":    "python",
	"ts":    "typescript",
	"pg":    "postgresql",
	"mongo": "mongodb",
}

// extractDocFilter inspects query tokens and returns a cleaned query
// and matched doc slugs. If no token matches a known doc, matchedSlugs is nil.
// If the caller already passed a non-empty callerDocSlug, auto-filter is skipped.
func extractDocFilter(query string, knownSlugs map[string]bool, callerDocSlug string) (cleanQuery string, matchedSlugs []string) {
	if callerDocSlug != "" {
		return query, nil
	}
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return query, nil
	}

	matchIdx := -1
	var matchedPrefix string

	for idx, tok := range tokens {
		t := strings.ToLower(tok)
		if alias, ok := docAliases[t]; ok {
			t = alias
		}
		for slug := range knownSlugs {
			sl := strings.ToLower(slug)
			if sl == t {
				matchIdx = idx
				matchedPrefix = t
				goto found
			}
			if tilde := strings.Index(sl, "~"); tilde >= 0 {
				pre := sl[:tilde]
				if pre == t {
					matchIdx = idx
					matchedPrefix = pre
					goto found
				}
				if dash := strings.Index(pre, "-"); dash >= 0 {
					base := pre[:dash]
					if base == t {
						matchIdx = idx
						matchedPrefix = base
						goto found
					}
				}
			} else {
				if dash := strings.Index(sl, "-"); dash >= 0 {
					base := sl[:dash]
					if base == t {
						matchIdx = idx
						matchedPrefix = base
						goto found
					}
				}
			}
		}
		continue
	found:
		break
	}

	if matchIdx < 0 {
		return query, nil
	}

	remaining := make([]string, 0, len(tokens)-1)
	for idx, tok := range tokens {
		if idx != matchIdx {
			remaining = append(remaining, tok)
		}
	}
	if len(remaining) == 0 {
		return query, nil
	}
	cleanQuery = strings.Join(remaining, " ")

	for slug := range knownSlugs {
		sl := strings.ToLower(slug)
		if sl == matchedPrefix {
			matchedSlugs = append(matchedSlugs, slug)
			continue
		}
		if tilde := strings.Index(sl, "~"); tilde >= 0 {
			pre := sl[:tilde]
			if pre == matchedPrefix {
				matchedSlugs = append(matchedSlugs, slug)
				continue
			}
			if dash := strings.Index(pre, "-"); dash >= 0 {
				if pre[:dash] == matchedPrefix {
					matchedSlugs = append(matchedSlugs, slug)
					continue
				}
			}
		} else {
			if dash := strings.Index(sl, "-"); dash >= 0 {
				if sl[:dash] == matchedPrefix {
					matchedSlugs = append(matchedSlugs, slug)
					continue
				}
			}
		}
	}
	return cleanQuery, matchedSlugs
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
	if sanitizeFTSQuery(query) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if docSlug != "" {
		return i.searchIn(ctx, query, []string{docSlug}, limit)
	}

	// Auto-detect doc filter from query tokens. A doc name in the query
	// may also be an ordinary word ("async function", "less than"), so
	// fall back to a global search when the filtered one finds nothing.
	slugMap, _, catErr := i.docCatalog(ctx)
	if catErr == nil && len(slugMap) > 0 {
		cleanQ, matched := extractDocFilter(query, slugMap, "")
		if len(matched) > 0 {
			hits, err := i.searchIn(ctx, cleanQ, matched, limit)
			if err != nil || len(hits) > 0 {
				return hits, err
			}
		}
	}
	return i.searchIn(ctx, query, nil, limit)
}

// searchIn runs the FTS search plus substring fallback. An empty
// docSlugs means a global search with the popularity boost.
func (i *Index) searchIn(ctx context.Context, query string, docSlugs []string, limit int) ([]SearchHit, error) {
	q := sanitizeFTSQuery(query)
	if q == "" {
		return nil, nil
	}
	hasDocFilter := len(docSlugs) > 0

	// Over-fetch when ranking globally so the popularity boost has
	// candidates to reorder. With a doc filter, no boost is applied
	// and the FTS LIMIT is enough.
	fetchLimit := limit
	if !hasDocFilter {
		fetchLimit = limit * 4
		if fetchLimit < 50 {
			fetchLimit = 50
		}
	}

	var (
		rows *sql.Rows
		err  error
	)

	if !hasDocFilter {
		rows, err = i.db.QueryContext(ctx, `
			SELECT e.doc_slug, e.name, e.path, e.type, bm25(entries_fts, 2.0, 1.0) AS score
			FROM entries_fts
			JOIN entries e ON e.id = entries_fts.rowid
			WHERE entries_fts MATCH ?
			ORDER BY score
			LIMIT ?`, q, fetchLimit)
	} else {
		placeholders := strings.Repeat("?,", len(docSlugs))
		placeholders = placeholders[:len(placeholders)-1]
		args := make([]any, 0, 2+len(docSlugs))
		args = append(args, q)
		for _, s := range docSlugs {
			args = append(args, s)
		}
		args = append(args, limit)
		rows, err = i.db.QueryContext(ctx,
			`SELECT e.doc_slug, e.name, e.path, e.type, bm25(entries_fts, 2.0, 1.0) AS score
			FROM entries_fts
			JOIN entries e ON e.id = entries_fts.rowid
			WHERE entries_fts MATCH ? AND e.doc_slug IN (`+placeholders+`)
			ORDER BY score
			LIMIT ?`,
			args...)
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
		if !hasDocFilter {
			h.Score -= popularityBoost(h.DocSlug)
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !hasDocFilter {
		// Re-sort and trim after applying the boost.
		sortHitsByScore(hits)
		if len(hits) > limit {
			hits = hits[:limit]
		}
	}

	// Augment with substring matches on entries.name only when the
	// FTS prefix-tokenizer didn't already saturate the result set.
	if len(hits) < limit/2 || len(hits) == 0 {
		fb, err := i.searchSubstring(ctx, query, docSlugs, limit)
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

// minSubstringQueryLen is the shortest query for which the global
// (unfiltered) '%q%' table scan is attempted. Shorter queries match far
// too much to be useful and the scan costs ~0.5 s on a large index.
const minSubstringQueryLen = 3

// searchSubstring is the LIKE-based fallback used to augment FTS
// prefix-match hits. docSlugs restricts results; empty/nil means no filter.
//
// Two passes: first 'tok%' on the leading token, which walks
// idx_entries_name and answers in milliseconds; only when that yields
// nothing does it fall back to the '%tok%' full scan, and only if the
// query is long enough or a doc filter bounds the scan.
func (i *Index) searchSubstring(ctx context.Context, query string, docSlugs []string, limit int) ([]SearchHit, error) {
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return nil, nil
	}

	// Determine effective filter: remove empty strings
	var filteredSlugs []string
	for _, s := range docSlugs {
		if s != "" {
			filteredSlugs = append(filteredSlugs, s)
		}
	}

	hits, err := i.likeQuery(ctx, tokens, filteredSlugs, limit, true)
	if err != nil || len(hits) > 0 {
		return hits, err
	}
	if len(filteredSlugs) == 0 && len(strings.Join(tokens, "")) < minSubstringQueryLen {
		return nil, nil
	}
	return i.likeQuery(ctx, tokens, filteredSlugs, limit, false)
}

// likeQuery runs one LIKE pass over entries. With prefix set the first
// token is matched as 'tok%' (index-friendly); otherwise every token is
// matched as '%tok%'. Remaining tokens are always '%tok%' filters.
func (i *Index) likeQuery(ctx context.Context, tokens, docSlugs []string, limit int, prefix bool) ([]SearchHit, error) {
	var clauses []string
	args := make([]any, 0, len(tokens)+len(docSlugs)+1)
	for n, t := range tokens {
		clauses = append(clauses, `name LIKE ? ESCAPE '\'`)
		if n == 0 && prefix {
			args = append(args, escapeLike(t)+"%")
		} else {
			args = append(args, "%"+escapeLike(t)+"%")
		}
	}
	where := strings.Join(clauses, " AND ")
	if len(docSlugs) > 0 {
		placeholders := strings.Repeat("?,", len(docSlugs))
		placeholders = placeholders[:len(placeholders)-1]
		where += " AND doc_slug IN (" + placeholders + ")"
		for _, s := range docSlugs {
			args = append(args, s)
		}
	}
	args = append(args, limit*4)

	rows, err := i.db.QueryContext(ctx,
		`SELECT doc_slug, name, path, type FROM entries WHERE `+where+` LIMIT ?`,
		args...)
	if err != nil {
		return nil, fmt.Errorf("search substring: %w", err)
	}
	defer rows.Close()

	noFilter := len(docSlugs) == 0
	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.DocSlug, &h.Name, &h.Path, &h.Type); err != nil {
			return nil, err
		}
		h.Score = float64(len(h.Name))
		if noFilter {
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
// query. Snippet is a short excerpt around the match
// (~20 tokens, with HTML-ish highlight markers). docSlug restricts the
// search to a single doc when non-empty.
func (i *Index) SearchPages(ctx context.Context, query, docSlug string, limit int) ([]PageHit, error) {
	if sanitizeFTSQuery(query) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if docSlug != "" {
		return i.searchPagesIn(ctx, query, []string{docSlug}, limit)
	}

	// Auto-detect doc filter from query tokens; fall back to a global
	// search when the filtered one finds nothing (see Search).
	slugMap, _, catErr := i.docCatalog(ctx)
	if catErr == nil && len(slugMap) > 0 {
		cleanQ, matched := extractDocFilter(query, slugMap, "")
		if len(matched) > 0 {
			hits, err := i.searchPagesIn(ctx, cleanQ, matched, limit)
			if err != nil || len(hits) > 0 {
				return hits, err
			}
		}
	}
	return i.searchPagesIn(ctx, query, nil, limit)
}

// searchPagesIn runs the page FTS search. An empty docSlugs means a
// global search with the popularity boost.
func (i *Index) searchPagesIn(ctx context.Context, query string, docSlugs []string, limit int) ([]PageHit, error) {
	q := sanitizeFTSQuery(query)
	if q == "" {
		return nil, nil
	}
	hasDocFilter := len(docSlugs) > 0

	var (
		rows *sql.Rows
		err  error
	)
	if !hasDocFilter {
		rows, err = i.db.QueryContext(ctx, `
			SELECT doc_slug, path,
			       snippet(pages_fts, 2, '[', ']', ' … ', 16) AS snip,
			       bm25(pages_fts) AS score
			FROM pages_fts
			WHERE pages_fts MATCH ?
			ORDER BY score
			LIMIT ?`, q, limit)
	} else {
		placeholders := strings.Repeat("?,", len(docSlugs))
		placeholders = placeholders[:len(placeholders)-1]
		args := make([]any, 0, 2+len(docSlugs))
		args = append(args, q)
		for _, s := range docSlugs {
			args = append(args, s)
		}
		args = append(args, limit)
		rows, err = i.db.QueryContext(ctx,
			`SELECT doc_slug, path,
			       snippet(pages_fts, 2, '[', ']', ' … ', 16) AS snip,
			       bm25(pages_fts) AS score
			FROM pages_fts
			WHERE pages_fts MATCH ? AND doc_slug IN (`+placeholders+`)
			ORDER BY score
			LIMIT ?`,
			args...)
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
		if !hasDocFilter {
			h.Score -= popularityBoost(h.DocSlug)
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !hasDocFilter {
		sortPageHitsByScore(hits)
	}
	return hits, nil
}

// sortPageHitsByScore sorts ascending (lower bm25 = better).
func sortPageHitsByScore(hits []PageHit) {
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j-1].Score > hits[j].Score; j-- {
			hits[j-1], hits[j] = hits[j], hits[j-1]
		}
	}
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
		escaped := strings.ReplaceAll(f, `"`, `""`)
		if escaped == "" {
			continue
		}
		parts = append(parts, `"`+escaped+`"*`)
	}
	return strings.Join(parts, " ")
}
