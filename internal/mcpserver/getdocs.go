package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/piligrim/marvindocs/internal/chunk"
	"github.com/piligrim/marvindocs/internal/docstore"
	"github.com/piligrim/marvindocs/internal/index"
)

// get_docs is the Context7-style entry point: one call with a topic and
// a token budget returns the most relevant Markdown fragments across
// the matching pages, instead of a list of hits to fetch one by one.

const (
	getDocsDefaultTokens = 5000
	getDocsMaxTokens     = 50000
	// getDocsPages is how many top pages contribute chunks.
	getDocsPages = 8
	// getDocsPerPage is how many best chunks one page contributes.
	getDocsPerPage = 3
	// getDocsMaxPageBytes skips pages larger than this (sans inline
	// images): they are generated indexes and tables of contents that
	// match every topic and cost hundreds of ms each to convert.
	getDocsMaxPageBytes = 512 << 10
)

type getDocsInput struct {
	Topic   string `json:"topic" jsonschema:"what to look up, e.g. 'jsonb operators' or 'useEffect cleanup'"`
	DocSlug string `json:"doc_slug,omitempty" jsonschema:"optional: restrict to this doc slug (see list_docs); otherwise detected from the topic"`
	Tokens  int    `json:"tokens,omitempty" jsonschema:"approximate token budget for the response (default 5000, max 50000)"`
}

// Fragment is one chunk of a page selected for the response.
type Fragment struct {
	DocSlug string `json:"doc_slug"`
	Path    string `json:"path"`
	Heading string `json:"heading,omitempty"`
	Content string `json:"content"`
	Tokens  int    `json:"tokens"`
}

type getDocsOutput struct {
	Topic     string     `json:"topic"`
	Tokens    int        `json:"tokens"`
	Fragments []Fragment `json:"fragments"`
}

func registerGetDocs(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_docs",
		Description: "Get documentation fragments about a topic within a token budget. " +
			"Searches ~1200-char Markdown chunks of every page, returns the best chunks " +
			"of the best pages, most relevant first, as Markdown with a 'Source:' " +
			"line each. Prefer this over " +
			"search_pages + get_page when you just need an answer.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getDocsInput) (*mcp.CallToolResult, getDocsOutput, error) {
		if strings.TrimSpace(in.Topic) == "" {
			return nil, getDocsOutput{}, fmt.Errorf("topic is required")
		}
		budget := in.Tokens
		if budget <= 0 {
			budget = getDocsDefaultTokens
		}
		if budget > getDocsMaxTokens {
			budget = getDocsMaxTokens
		}
		slug, err := d.resolveSlug(ctx, in.DocSlug)
		if err != nil {
			return nil, getDocsOutput{}, err
		}
		frags, err := d.collectFragments(ctx, in.Topic, slug, budget)
		if err != nil {
			return nil, getDocsOutput{}, err
		}
		out := getDocsOutput{Topic: in.Topic, Fragments: frags}
		var text strings.Builder
		for k, f := range frags {
			out.Tokens += f.Tokens
			if k > 0 {
				text.WriteString("\n\n---\n\n")
			}
			fmt.Fprintf(&text, "Source: %s/%s\n\n%s", f.DocSlug, f.Path, f.Content)
		}
		if len(frags) == 0 {
			fmt.Fprintf(&text, "no documentation found for %q", in.Topic)
		}
		return textResult(text.String()), out, nil
	})
}

// collectFragments runs the chunk search, re-slices the chunks from the
// Markdown of their pages and fills the token budget best-first. A chunk
// that does not fit is truncated and ends the selection.
func (d Deps) collectFragments(ctx context.Context, topic, docSlug string, budget int) ([]Fragment, error) {
	hits, err := d.searchChunks(ctx, topic, docSlug)
	if err != nil {
		return nil, err
	}

	// Distinct pages in search order.
	type pageKey struct{ slug, path string }
	var pages []pageKey
	pageIdx := make(map[pageKey]int)
	for _, h := range hits {
		k := pageKey{h.DocSlug, h.Path}
		if _, ok := pageIdx[k]; !ok {
			pageIdx[k] = len(pages)
			pages = append(pages, k)
		}
	}

	// HTML to Markdown dominates a cold call (large pages take ~200 ms
	// each), so convert the pages concurrently; the result order is kept.
	mds := make([]string, len(pages))
	errs := make([]error, len(pages))
	var wg sync.WaitGroup
	for i, p := range pages {
		wg.Add(1)
		go func() {
			defer wg.Done()
			html, err := d.Store.Page(p.slug, p.path)
			if err != nil {
				return // stale index or missing db.json
			}
			html = docstore.StripDataURIs(html)
			if len(html) > getDocsMaxPageBytes {
				return
			}
			mds[i], errs[i] = d.markdown(p.slug, p.path, html)
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var cands []Fragment
	for _, h := range hits {
		i := pageIdx[pageKey{h.DocSlug, h.Path}]
		if errs[i] != nil {
			return nil, errs[i]
		}
		md := mds[i]
		if h.Off < 0 || h.Len <= 0 || h.Off+h.Len > len(md) {
			continue // stale index or skipped page
		}
		cands = append(cands, Fragment{
			DocSlug: h.DocSlug, Path: h.Path,
			Heading: chunk.HeadingAt(md, h.Off),
			Content: md[h.Off : h.Off+h.Len],
		})
	}

	frags := []Fragment{}
	left := budget
	for _, f := range cands {
		if left <= 0 {
			break
		}
		f.Content = chunk.Truncate(f.Content, left)
		f.Tokens = chunk.Tokens(f.Content)
		if f.Tokens == 0 {
			break
		}
		frags = append(frags, f)
		left -= f.Tokens
	}
	return frags, nil
}

// searchChunks runs the hybrid chunk search when an embedder is set and
// the doc has vectors, else the lexical one.
func (d Deps) searchChunks(ctx context.Context, topic, docSlug string) ([]index.ChunkHit, error) {
	if d.Embedder != nil && docSlug != "" {
		has, err := d.Index.HasVectors(ctx, docSlug)
		if err != nil {
			return nil, err
		}
		if has {
			vecs, err := d.Embedder.Embed(ctx, []string{topic})
			if err != nil {
				return nil, err
			}
			return d.Index.SearchChunksHybrid(ctx, vecs[0], topic, docSlug, getDocsPages, getDocsPerPage)
		}
	}
	return d.Index.SearchChunks(ctx, topic, docSlug, getDocsPages, getDocsPerPage)
}
