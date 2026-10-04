package index

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/piligrim/marvindocs/internal/embed"
)

// docVecs holds the quantised chunk vectors of one doc in memory, as
// parallel slices. Entries are loaded lazily and dropped by EmbedDoc and
// Reset; a Sync from another process is not noticed until restart.
type docVecs struct {
	ids, pageIDs []int64
	offs, lens   []int
	vecs         [][]byte
}

// dropVecCache forgets the cached vectors of slug.
func (i *Index) dropVecCache(slug string) {
	i.vecMu.Lock()
	delete(i.vecCache, slug)
	i.vecMu.Unlock()
}

// docVectors returns the cached vectors of slug, loading them on first use.
func (i *Index) docVectors(ctx context.Context, slug string) (*docVecs, error) {
	i.vecMu.Lock()
	defer i.vecMu.Unlock()
	if dv, ok := i.vecCache[slug]; ok {
		return dv, nil
	}
	rows, err := i.db.QueryContext(ctx,
		`SELECT c.id, c.page_id, c.off, c.len, v.vec FROM chunk_vecs v
		 JOIN chunks c ON c.id = v.chunk_id JOIN pages p ON p.id = c.page_id
		 WHERE p.doc_slug = ?`, slug)
	if err != nil {
		return nil, fmt.Errorf("load vectors %s: %w", slug, err)
	}
	defer rows.Close()
	dv := &docVecs{}
	for rows.Next() {
		var id, pageID int64
		var off, n int
		var vec []byte
		if err := rows.Scan(&id, &pageID, &off, &n, &vec); err != nil {
			return nil, err
		}
		dv.ids = append(dv.ids, id)
		dv.pageIDs = append(dv.pageIDs, pageID)
		dv.offs = append(dv.offs, off)
		dv.lens = append(dv.lens, n)
		dv.vecs = append(dv.vecs, vec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if i.vecCache == nil {
		i.vecCache = make(map[string]*docVecs)
	}
	i.vecCache[slug] = dv
	return dv, nil
}

// HasVectors reports whether slug has at least one stored chunk vector.
func (i *Index) HasVectors(ctx context.Context, slug string) (bool, error) {
	i.vecMu.Lock()
	dv, ok := i.vecCache[slug]
	i.vecMu.Unlock()
	if ok {
		return len(dv.ids) > 0, nil
	}
	var has bool
	err := i.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM chunk_vecs v
		 JOIN chunks c ON c.id = v.chunk_id JOIN pages p ON p.id = c.page_id
		 WHERE p.doc_slug = ?)`, slug).Scan(&has)
	return has, err
}

// vecScore is one scored chunk: slug and row index into the doc's docVecs.
type vecScore struct {
	slug string
	idx  int
	id   int64
	cos  float32
}

// worse reports whether a ranks below b (lower cosine, then higher id).
func (a vecScore) worse(b vecScore) bool {
	if a.cos != b.cos {
		return a.cos < b.cos
	}
	return a.id > b.id
}

// topHeap is a min-heap on rank: the worst kept chunk is on top.
type topHeap []vecScore

func (h topHeap) Len() int           { return len(h) }
func (h topHeap) Less(a, b int) bool { return h[a].worse(h[b]) }
func (h topHeap) Swap(a, b int)      { h[a], h[b] = h[b], h[a] }
func (h *topHeap) Push(x any)        { *h = append(*h, x.(vecScore)) }
func (h *topHeap) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}

// topVecChunks returns the k chunks of docSlugs closest to q (brute
// force over the cached vectors), best first. score is -cosine, so
// lower = better; path is left empty (see fillPaths).
func (i *Index) topVecChunks(ctx context.Context, q []float32, docSlugs []string, k int) ([]chunkCand, error) {
	if k <= 0 {
		return nil, nil
	}
	h := make(topHeap, 0, k+1)
	vecsBySlug := make(map[string]*docVecs, len(docSlugs))
	for _, slug := range docSlugs {
		dv, err := i.docVectors(ctx, slug)
		if err != nil {
			return nil, err
		}
		vecsBySlug[slug] = dv
		for n, blob := range dv.vecs {
			s := vecScore{slug: slug, idx: n, id: dv.ids[n], cos: embed.Dot(q, blob)}
			if len(h) < k {
				heap.Push(&h, s)
			} else if h[0].worse(s) {
				h[0] = s
				heap.Fix(&h, 0)
			}
		}
	}
	top := []vecScore(h)
	sort.Slice(top, func(a, b int) bool { return top[b].worse(top[a]) })

	cands := make([]chunkCand, 0, len(top))
	for _, s := range top {
		dv := vecsBySlug[s.slug]
		cands = append(cands, chunkCand{
			docSlug: s.slug, pageID: dv.pageIDs[s.idx], off: dv.offs[s.idx], len: dv.lens[s.idx],
			score: -float64(s.cos),
		})
	}
	return cands, nil
}

// cutPages keeps the first pages pages of perPage chunks of the
// score-ordered cands, in queryChunks order, and sets best.
func cutPages(cands []chunkCand, pages, perPage int) []chunkCand {
	groups := groupByPage(cands)
	if len(groups) > pages {
		groups = groups[:pages]
	}
	var out []chunkCand
	for _, g := range groups {
		if len(g) > perPage {
			g = g[:perPage]
		}
		for _, c := range g {
			c.best = g[0].score
			out = append(out, c)
		}
	}
	return out
}

// searchChunksVec ranks the chunks of docSlugs by cosine to q and
// returns them like queryChunks: best page first, then best chunk
// first, at most pages pages and perPage chunks per page.
func (i *Index) searchChunksVec(ctx context.Context, q []float32, docSlugs []string, pages, perPage int) ([]chunkCand, error) {
	cands, err := i.topVecChunks(ctx, q, docSlugs, pages*perPage*4)
	if err != nil {
		return nil, err
	}
	out := cutPages(cands, pages, perPage)
	return out, i.fillPaths(ctx, out)
}

// Rank fusion: a chunk scores 1/(rrfK+rank) for its vector rank plus
// lexWeight/(rrfK+rank) for its lexical rank; a missing rank adds
// nothing. The vector side leads because it handles paraphrases, the
// lexical side rescues exact names. On the 32-query bench the plateau
// is lexWeight 0.5..1 (hit@1 30/32 against 25/32 for either alone).
const (
	rrfK      = 60
	lexWeight = 0.5
)

// searchChunksHybrid fuses vector and lexical chunk ranks of docSlugs
// and returns them like queryChunks. score is the negated fused score.
func (i *Index) searchChunksHybrid(ctx context.Context, q []float32, query string, docSlugs []string, pages, perPage int) ([]chunkCand, error) {
	k := pages * perPage * 4
	vec, err := i.topVecChunks(ctx, q, docSlugs, k)
	if err != nil {
		return nil, err
	}
	lex, err := i.searchChunksIn(ctx, query, docSlugs, pages*4, perPage)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(lex, func(a, b int) bool { return lex[a].score < lex[b].score })

	type key struct {
		pageID int64
		off    int
	}
	fused := make(map[key]*chunkCand, len(vec)+len(lex))
	var order []key
	add := func(c chunkCand, rank int, w float64) {
		k := key{c.pageID, c.off}
		f, ok := fused[k]
		if !ok {
			c.score = 0
			fused[k] = &c
			order = append(order, k)
			f = &c
		}
		if f.path == "" {
			f.path = c.path
		}
		f.score -= w / float64(rrfK+rank)
	}
	for n, c := range vec {
		add(c, n+1, 1)
	}
	for n, c := range lex {
		add(c, n+1, lexWeight)
	}
	cands := make([]chunkCand, 0, len(order))
	for _, k := range order {
		cands = append(cands, *fused[k])
	}
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].score < cands[b].score })
	out := cutPages(cands, pages, perPage)
	return out, i.fillPaths(ctx, out)
}

// fillPaths sets the path of every candidate from its page row.
func (i *Index) fillPaths(ctx context.Context, cands []chunkCand) error {
	seen := make(map[int64]bool)
	var args []any
	for _, c := range cands {
		if c.path == "" && !seen[c.pageID] {
			seen[c.pageID] = true
			args = append(args, c.pageID)
		}
	}
	if len(args) == 0 {
		return nil
	}
	rows, err := i.db.QueryContext(ctx,
		`SELECT id, path FROM pages WHERE id IN (`+strings.Repeat("?,", len(args)-1)+`?)`, args...)
	if err != nil {
		return fmt.Errorf("resolve pages: %w", err)
	}
	defer rows.Close()
	paths := make(map[int64]string, len(args))
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			return err
		}
		paths[id] = path
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for n := range cands {
		if cands[n].path == "" {
			cands[n].path = paths[cands[n].pageID]
		}
	}
	return nil
}

// SearchChunksVec mirrors SearchChunks but ranks by the query vector q.
// docSlug must be non-empty: vector search is per doc.
func (i *Index) SearchChunksVec(ctx context.Context, q []float32, docSlug string, pages, perPage int) ([]ChunkHit, error) {
	if docSlug == "" {
		return nil, errors.New("vector search needs a doc slug")
	}
	cands, err := i.searchChunksVec(ctx, q, []string{docSlug}, pages, perPage)
	if err != nil {
		return nil, err
	}
	return chunkHits(cands, pages, perPage), nil
}

// SearchPagesVec mirrors SearchPages but ranks by the query vector q;
// query is only used to build the snippets. docSlug must be non-empty.
func (i *Index) SearchPagesVec(ctx context.Context, pages PageSource, q []float32, query, docSlug string, limit int) ([]PageHit, error) {
	if docSlug == "" {
		return nil, errors.New("vector search needs a doc slug")
	}
	if limit <= 0 {
		limit = 20
	}
	cands, err := i.searchChunksVec(ctx, q, []string{docSlug}, limit, 1)
	if err != nil {
		return nil, err
	}
	return pageHits(ctx, pages, cands, query, limit)
}

// SearchChunksHybrid mirrors SearchChunks but ranks by the query vector
// q fused with the lexical ranking of query. docSlug must be non-empty.
func (i *Index) SearchChunksHybrid(ctx context.Context, q []float32, query, docSlug string, pages, perPage int) ([]ChunkHit, error) {
	if docSlug == "" {
		return nil, errors.New("vector search needs a doc slug")
	}
	cands, err := i.searchChunksHybrid(ctx, q, query, []string{docSlug}, pages, perPage)
	if err != nil {
		return nil, err
	}
	return chunkHits(cands, pages, perPage), nil
}
