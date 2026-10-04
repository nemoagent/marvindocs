package index

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/piligrim/marvindocs/internal/docstore"
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
	return cleanQuery, newestPerDoc(matchedSlugs)
}

// newestPerDoc keeps one slug per doc: when several versions of the same
// doc match (python~2.7, python~3.13) only the newest is returned, so an
// auto-detected filter behaves like "latest" instead of mixing versions.
// Slugs without a version are kept as-is. The result is sorted.
func newestPerDoc(slugs []string) []string {
	best := make(map[string]string, len(slugs))
	for _, s := range slugs {
		key := s
		if tilde := strings.Index(s, "~"); tilde >= 0 {
			key = s[:tilde]
		}
		if cur, ok := best[key]; !ok || compareVersions(s, cur) > 0 {
			best[key] = s
		}
	}
	out := make([]string, 0, len(best))
	for _, s := range best {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// compareVersions orders two slugs of the same doc by the version after
// "~", comparing dot-separated segments numerically (17 > 9.4, 3.13 > 3.9).
// Non-numeric segments fall back to string order. A slug without "~" is
// the current release in DevDocs and always sorts newest.
func compareVersions(a, b string) int {
	va, vb := versionOf(a), versionOf(b)
	switch {
	case va == "" && vb != "":
		return 1
	case va != "" && vb == "":
		return -1
	}
	for len(va) > 0 || len(vb) > 0 {
		var sa, sb string
		sa, va = nextSegment(va)
		sb, vb = nextSegment(vb)
		na, errA := strconv.Atoi(leadingDigits(sa))
		nb, errB := strconv.Atoi(leadingDigits(sb))
		switch {
		case errA == nil && errB == nil && na != nb:
			if na > nb {
				return 1
			}
			return -1
		case errA == nil && errB != nil:
			return 1
		case errA != nil && errB == nil:
			return -1
		case sa != sb:
			return strings.Compare(sa, sb)
		}
	}
	return 0
}

func versionOf(slug string) string {
	if tilde := strings.Index(slug, "~"); tilde >= 0 {
		return slug[tilde+1:]
	}
	return ""
}

func nextSegment(v string) (seg, rest string) {
	if dot := strings.IndexAny(v, "._-"); dot >= 0 {
		return v[:dot], v[dot+1:]
	}
	return v, ""
}

func leadingDigits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
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

// PageSource returns the HTML of a page; *docstore.Store satisfies it.
// SearchPages uses it to rebuild snippets, since chunks_fts stores no text.
type PageSource interface {
	Page(slug, path string) (string, error)
}

// chunkCand is one chunk matched by the FTS query.
type chunkCand struct {
	docSlug, path string
	pageID        int64
	off, len      int
	score         float64 // lower = better (bm25, minus popularity boost)
	best          float64 // score of the page's best chunk
}

// pagePool is how many pages are fetched before the popularity boost
// re-sorts them to pick n: the boost can promote a page from below.
func pagePool(n int) int {
	return max(n*4, 40)
}

// SearchPages queries chunks_fts for pages with a chunk matching the
// query, ranked by their best chunk. docSlug restricts the search to a
// single doc when non-empty. When pages is non-nil, each hit's Snippet
// is a short excerpt of the best chunk around the first matching word
// (matches wrapped in [ ]).
func (i *Index) SearchPages(ctx context.Context, pages PageSource, query, docSlug string, limit int) ([]PageHit, error) {
	if limit <= 0 {
		limit = 20
	}
	cands, err := i.searchChunksAuto(ctx, query, docSlug, pagePool(limit), 1)
	if err != nil {
		return nil, err
	}
	groups := groupByPage(cands)
	if len(groups) > limit {
		groups = groups[:limit]
	}
	var hits []PageHit
	for _, g := range groups {
		best := g[0]
		h := PageHit{DocSlug: best.docSlug, Path: best.path, Score: best.score}
		if pages != nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			h.Snippet = pageSnippet(pages, best, query)
		}
		hits = append(hits, h)
	}
	return hits, nil
}

// pageSnippet builds the snippet for page hit c from its best chunk.
// Returns "" for a stale index or a missing db.json.
func pageSnippet(pages PageSource, c chunkCand, query string) string {
	html, err := pages.Page(c.docSlug, c.path)
	if err != nil {
		return ""
	}
	md, err := docstore.ToMarkdown(html)
	if err != nil {
		return ""
	}
	text := md
	if c.off >= 0 && c.len >= 0 && c.off+c.len <= len(md) {
		text = md[c.off : c.off+c.len]
	}
	return snippet(text, query)
}

// ChunkHit is one matching chunk; Off/Len address the page's Markdown.
type ChunkHit struct {
	DocSlug string
	Path    string
	Heading string // not filled by the index; see chunk.HeadingAt
	Off     int
	Len     int
	Score   float64 // lower = better
}

// SearchChunks returns the best chunks for query, at most perPage per
// page and at most pages distinct pages, ordered by page relevance then
// chunk score. docSlug restricts the search like in SearchPages.
func (i *Index) SearchChunks(ctx context.Context, query, docSlug string, pages, perPage int) ([]ChunkHit, error) {
	cands, err := i.searchChunksAuto(ctx, query, docSlug, pagePool(pages), perPage)
	if err != nil {
		return nil, err
	}
	groups := groupByPage(cands)
	if len(groups) > pages {
		groups = groups[:pages]
	}
	var hits []ChunkHit
	for _, g := range groups {
		if len(g) > perPage {
			g = g[:perPage]
		}
		for _, c := range g {
			hits = append(hits, ChunkHit{DocSlug: c.docSlug, Path: c.path, Off: c.off, Len: c.len, Score: c.score})
		}
	}
	return hits, nil
}

// groupByPage groups score-ordered candidates by page. Pages come in
// order of their best chunk; chunks within a page keep their order.
func groupByPage(cands []chunkCand) [][]chunkCand {
	var groups [][]chunkCand
	byPage := make(map[int64]int)
	for _, c := range cands {
		g, ok := byPage[c.pageID]
		if !ok {
			g = len(groups)
			byPage[c.pageID] = g
			groups = append(groups, nil)
		}
		groups[g] = append(groups[g], c)
	}
	return groups
}

// searchChunksAuto applies the doc filter: explicit docSlug, else one
// auto-detected from the query tokens with a global fallback (see Search).
func (i *Index) searchChunksAuto(ctx context.Context, query, docSlug string, pages, perPage int) ([]chunkCand, error) {
	if docSlug != "" {
		return i.searchChunksIn(ctx, query, []string{docSlug}, pages, perPage)
	}
	slugMap, _, catErr := i.docCatalog(ctx)
	if catErr == nil && len(slugMap) > 0 {
		cleanQ, matched := extractDocFilter(query, slugMap, "")
		if len(matched) > 0 {
			cands, err := i.searchChunksIn(ctx, cleanQ, matched, pages, perPage)
			if err != nil || len(cands) > 0 {
				return cands, err
			}
		}
	}
	return i.searchChunksIn(ctx, query, nil, pages, perPage)
}

// searchChunksIn runs the chunk FTS search, first requiring every word
// (with prefix matching), then falling back to any-word matching.
func (i *Index) searchChunksIn(ctx context.Context, query string, docSlugs []string, pages, perPage int) ([]chunkCand, error) {
	q := sanitizeFTSQuery(query)
	if q == "" {
		return nil, nil
	}
	cands, err := i.queryChunks(ctx, q, docSlugs, pages, perPage)
	if err != nil || len(cands) > 0 || len(strings.Fields(query)) < 2 {
		return cands, err
	}
	// Natural-language queries rarely have every word in one chunk;
	// fall back to any-word matching and let bm25 rank by overlap.
	return i.queryChunks(ctx, anyWordFTSQuery(query), docSlugs, pages, perPage)
}

// queryChunks runs one FTS MATCH and returns the top perPage chunks of
// the top pages (ranked by their best chunk), best page first, chunks
// within a page best first. The crumb column weighs three times the
// body. An empty docSlugs means a global search with the popularity
// boost, which re-sorts the pages by their adjusted best score.
func (i *Index) queryChunks(ctx context.Context, q string, docSlugs []string, pages, perPage int) ([]chunkCand, error) {
	hasDocFilter := len(docSlugs) > 0

	sqlText := `WITH m AS (
			SELECT rowid AS rid, rank FROM chunks_fts
			WHERE chunks_fts MATCH ? AND rank MATCH 'bm25(3.0, 1.0)'
		), r AS (
			SELECT c.page_id, p.doc_slug, p.path, c.off, c.len, m.rank,
				ROW_NUMBER() OVER w AS rn, MIN(m.rank) OVER w AS best
			FROM m JOIN chunks c ON c.id = m.rid JOIN pages p ON p.id = c.page_id`
	args := []any{q}
	if hasDocFilter {
		placeholders := strings.Repeat("?,", len(docSlugs))
		sqlText += ` WHERE p.doc_slug IN (` + placeholders[:len(placeholders)-1] + `)`
		for _, s := range docSlugs {
			args = append(args, s)
		}
	}
	sqlText += `
			WINDOW w AS (PARTITION BY c.page_id ORDER BY m.rank)
		)
		SELECT page_id, doc_slug, path, off, len, rank, best FROM r
		WHERE rn <= ? ORDER BY best, page_id, rn LIMIT ?`
	args = append(args, perPage, pages*perPage)

	rows, err := i.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("search chunks: %w", err)
	}
	defer rows.Close()

	var cands []chunkCand
	for rows.Next() {
		var c chunkCand
		if err := rows.Scan(&c.pageID, &c.docSlug, &c.path, &c.off, &c.len, &c.score, &c.best); err != nil {
			return nil, err
		}
		if !hasDocFilter {
			boost := popularityBoost(c.docSlug)
			c.score -= boost
			c.best -= boost
		}
		cands = append(cands, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !hasDocFilter {
		sort.SliceStable(cands, func(a, b int) bool {
			if cands[a].best != cands[b].best {
				return cands[a].best < cands[b].best
			}
			return cands[a].pageID < cands[b].pageID
		})
	}
	return cands, nil
}

// Snippet window, in words, around the first match: mirrors the 16-token
// excerpt FTS5's snippet() used to produce.
const (
	snippetBefore = 6
	snippetWords  = 24
)

// snippet returns a short excerpt of text around the first word that
// starts with any of the query tokens (the FTS query is prefix-based,
// so this is the same notion of a match). Matching words are wrapped
// in [ ]; a leading/trailing "…" marks truncation. Falls back to the
// beginning of the text when nothing matches, e.g. for diacritic folds.
func snippet(text, query string) string {
	tokens := strings.Fields(strings.ToLower(query))
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	matches := func(w string) bool {
		lw := strings.ToLower(strings.TrimLeft(w, "([{\"'`"))
		for _, t := range tokens {
			if strings.HasPrefix(lw, t) {
				return true
			}
		}
		return false
	}
	first := 0
	for k, w := range words {
		if matches(w) {
			first = k
			break
		}
	}
	start := max(first-snippetBefore, 0)
	end := min(start+snippetWords, len(words))

	var b strings.Builder
	if start > 0 {
		b.WriteString("… ")
	}
	for k := start; k < end; k++ {
		if k > start {
			b.WriteByte(' ')
		}
		if matches(words[k]) {
			b.WriteString("[" + words[k] + "]")
		} else {
			b.WriteString(words[k])
		}
	}
	if end < len(words) {
		b.WriteString(" …")
	}
	return b.String()
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
// anyWordFTSQuery builds an OR query of exact (non-prefix) tokens for
// the natural-language fallback: prefixes on short words like "a" or
// "in" would match nearly everything.
func anyWordFTSQuery(q string) string {
	fields := strings.Fields(q)
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		parts = append(parts, `"`+strings.ReplaceAll(f, `"`, `""`)+`"`)
	}
	return strings.Join(parts, " OR ")
}

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
