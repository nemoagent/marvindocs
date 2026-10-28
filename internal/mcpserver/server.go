// Package mcpserver exposes the marvindocs data to MCP clients (LLM agents).
//
// Five tools are registered:
//
//	list_docs    -> find documentation sets by name
//	search       -> FTS5 search over entry names and paths
//	search_pages -> FTS5 search over page bodies, with snippets
//	get_page     -> fetch a page as Markdown (default) or raw HTML
//	get_docs     -> topic + token budget -> relevant Markdown fragments
//
// The server works over both stdio (for Claude Desktop, Cursor, Claude Code)
// and Streamable HTTP (for remote use). Both transports share one Server
// instance and therefore one index + docstore handle.
package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/piligrim/marvindocs/internal/docstore"
	"github.com/piligrim/marvindocs/internal/embed"
	"github.com/piligrim/marvindocs/internal/index"
	"github.com/piligrim/marvindocs/internal/mdcache"
)

// Deps is the bundle of state the MCP tools need. Constructed by the CLI
// once and passed to New.
type Deps struct {
	Store *docstore.Store
	Index *index.Index
	// MDCache caches HTML→Markdown conversions across get_page calls.
	// Optional: a nil cache is treated as disabled.
	MDCache *mdcache.Cache
	// Embedder turns the topic of get_docs into a vector for the hybrid
	// chunk search. Optional: nil keeps the lexical search only.
	Embedder embed.Embedder
}

// instructions tell the agent what the server is for; clients that support
// MCP server instructions add them to the system prompt.
const instructions = "Reference documentation for hundreds of languages, frameworks, " +
	"libraries and tools (React, Django, PostgreSQL, Go, Python, Docker, ...), " +
	"often in several versions. Before answering from memory about an API, " +
	"option or behaviour, call get_docs with the topic. To pin a library or " +
	"version, find its slug with list_docs(name) and pass it as doc_slug."

// New builds a configured mcp.Server with all marvindocs tools registered.
// Version identifies the server to clients.
func New(deps Deps, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "marvindocs",
		Version: version,
	}, &mcp.ServerOptions{Instructions: instructions})
	registerListDocs(s, deps)
	registerSearch(s, deps)
	registerSearchPages(s, deps)
	registerGetPage(s, deps)
	registerGetDocs(s, deps)
	return s
}

// -------------------------------------------------------------------------
// list_docs
// -------------------------------------------------------------------------

type listDocsInput struct {
	Name string `json:"name,omitempty" jsonschema:"library or tool to look for, e.g. 'react', 'postgres', 'django'; matched as a substring of the slug and name"`
}

type listDocsOutput struct {
	Docs []index.DocRow `json:"docs"`
}

func registerListDocs(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_docs",
		Description: "Find documentation sets for a library, language or tool by name " +
			"and get their slugs and versions (e.g. name='django' -> django~5.2, django~4.2, ...). " +
			"Use the slug as doc_slug in get_docs, search or get_page to pin a version. " +
			"Not needed for a plain question: get_docs detects the documentation from the topic.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listDocsInput) (*mcp.CallToolResult, listDocsOutput, error) {
		docs, err := d.Index.ListDocs(ctx)
		if err != nil {
			return nil, listDocsOutput{}, err
		}
		name := strings.ToLower(strings.TrimSpace(in.Name))
		if name == "" {
			return textResult(fmt.Sprintf("%d documentation sets indexed (languages, "+
					"frameworks, libraries, tools). Pass name to find one, e.g. name='react'.", len(docs))),
				listDocsOutput{Docs: []index.DocRow{}}, nil
		}
		matched := []index.DocRow{}
		for _, doc := range docs {
			if strings.Contains(strings.ToLower(doc.Slug), name) ||
				strings.Contains(strings.ToLower(doc.Name), name) {
				matched = append(matched, doc)
			}
		}
		docs = matched
		return textResult(formatDocs(docs)),
			listDocsOutput{Docs: docs}, nil
	})
}

// -------------------------------------------------------------------------
// search
// -------------------------------------------------------------------------

type searchInput struct {
	Query   string `json:"query" jsonschema:"the search query (names and paths, prefix-matched)"`
	DocSlug string `json:"doc_slug,omitempty" jsonschema:"optional: restrict results to this doc slug"`
	Limit   int    `json:"limit,omitempty" jsonschema:"max number of hits (default 20, max 100)"`
}

type searchOutput struct {
	Hits []index.SearchHit `json:"hits"`
}

func registerSearch(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "search",
		Description: "Search across all indexed documentation. Matches entry names and paths. " +
			"Results are ranked by BM25; lower score = better match. " +
			"Pass doc_slug to restrict results to a single documentation set. " +
			"When no entry matches, falls back to full-text search over page " +
			"bodies: such hits have type 'page' and a snippet in 'name'.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, searchOutput, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}
		slug, err := d.resolveSlug(ctx, in.DocSlug)
		if err != nil {
			return nil, searchOutput{}, err
		}
		hits, err := d.Index.Search(ctx, in.Query, slug, limit)
		if err != nil {
			return nil, searchOutput{}, err
		}
		if len(hits) == 0 {
			hits, err = searchPagesFallback(ctx, d, in.Query, slug, limit)
			if err != nil {
				return nil, searchOutput{}, err
			}
		}
		if hits == nil {
			hits = []index.SearchHit{}
		}
		return textResult(formatHits(in.Query, hits)),
			searchOutput{Hits: hits}, nil
	})
}

// searchPagesFallback runs the full-text page search and presents the
// hits in the search tool's shape, so an agent whose query names a
// concept rather than a symbol gets results without a second call.
func searchPagesFallback(ctx context.Context, d Deps, query, docSlug string, limit int) ([]index.SearchHit, error) {
	pages, err := d.Index.SearchPages(ctx, d.Store, query, docSlug, limit)
	if err != nil {
		return nil, err
	}
	hits := make([]index.SearchHit, 0, len(pages))
	for _, p := range pages {
		hits = append(hits, index.SearchHit{
			DocSlug: p.DocSlug,
			Name:    p.Snippet,
			Path:    p.Path,
			Type:    "page",
			Score:   p.Score,
		})
	}
	return hits, nil
}

// -------------------------------------------------------------------------
// search_pages
// -------------------------------------------------------------------------

type searchPagesInput struct {
	Query   string `json:"query" jsonschema:"the search query (matched against full page text)"`
	DocSlug string `json:"doc_slug,omitempty" jsonschema:"optional: restrict results to this doc slug"`
	Limit   int    `json:"limit,omitempty" jsonschema:"max number of hits (default 20, max 100)"`
}

type searchPagesOutput struct {
	Hits []index.PageHit `json:"hits"`
}

func registerSearchPages(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "search_pages",
		Description: "Full-text search across page bodies (not just symbol names). " +
			"Use this when the 'search' tool returns no hits because your query " +
			"references concepts that aren't entry names — e.g. 'jsonb operators', " +
			"'connection pool size'. Each hit includes a snippet around the match.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchPagesInput) (*mcp.CallToolResult, searchPagesOutput, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}
		slug, err := d.resolveSlug(ctx, in.DocSlug)
		if err != nil {
			return nil, searchPagesOutput{}, err
		}
		hits, err := d.Index.SearchPages(ctx, d.Store, in.Query, slug, limit)
		if err != nil {
			return nil, searchPagesOutput{}, err
		}
		if hits == nil {
			hits = []index.PageHit{}
		}
		return textResult(formatPageHits(in.Query, hits)),
			searchPagesOutput{Hits: hits}, nil
	})
}

// -------------------------------------------------------------------------
// get_page
// -------------------------------------------------------------------------

type getPageInput struct {
	DocSlug string `json:"doc_slug" jsonschema:"slug of the documentation set (see list_docs)"`
	Path    string `json:"path" jsonschema:"path of the page within the doc (see search.path)"`
	Format  string `json:"format,omitempty" jsonschema:"'markdown' (default) or 'html'"`
}

type getPageOutput struct {
	DocSlug string `json:"doc_slug"`
	Path    string `json:"path"`
	Format  string `json:"format"`
	Content string `json:"content"`
}

func registerGetPage(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_page",
		Description: "Fetch a documentation page by (doc_slug, path). " +
			"Returns Markdown by default ('format' = 'markdown') or raw HTML ('format' = 'html').",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getPageInput) (*mcp.CallToolResult, getPageOutput, error) {
		if in.DocSlug == "" || in.Path == "" {
			return nil, getPageOutput{}, fmt.Errorf("doc_slug and path are required")
		}
		html, err := d.Store.Page(in.DocSlug, in.Path)
		if err != nil {
			return nil, getPageOutput{}, err
		}

		format := strings.ToLower(strings.TrimSpace(in.Format))
		if format == "" {
			format = "markdown"
		}
		out := getPageOutput{DocSlug: in.DocSlug, Path: in.Path, Format: format}
		switch format {
		case "html":
			out.Content = html
		case "markdown", "md":
			out.Format = "markdown"
			md, err := d.markdown(in.DocSlug, in.Path, html)
			if err != nil {
				return nil, getPageOutput{}, err
			}
			out.Content = md
		default:
			return nil, getPageOutput{}, fmt.Errorf("unknown format %q (use 'markdown' or 'html')", in.Format)
		}
		return textResult(out.Content), out, nil
	})
}

// -------------------------------------------------------------------------
// helpers
// -------------------------------------------------------------------------

// markdown converts a page's HTML to Markdown, going through MDCache.
func (d Deps) markdown(slug, path, html string) (string, error) {
	doc := d.Store.Doc(slug)
	var release string
	if doc != nil {
		release = doc.Release
	}
	key := mdcache.Key(slug, path, release)
	if md, ok := d.MDCache.Get(key); ok {
		return md, nil
	}
	md, err := docstore.ToMarkdown(html)
	if err != nil {
		return "", fmt.Errorf("convert to markdown: %w", err)
	}
	d.MDCache.Add(key, md)
	return md, nil
}

// The format* helpers render tool results as the text block: many MCP
// clients pass only content (not structuredContent) to the model, so the
// text must carry the data itself, one compact line per item.

func formatDocs(docs []index.DocRow) string {
	var b strings.Builder
	if len(docs) == 0 {
		return "No documentation set matches this name. Try a shorter or different name, " +
			"or call get_docs with the topic directly."
	}
	fmt.Fprintf(&b, "%d documentation sets (slug | name | version):\n", len(docs))
	for _, d := range docs {
		fmt.Fprintf(&b, "%s | %s | %s\n", d.Slug, d.Name, d.Version)
	}
	return b.String()
}

func formatHits(query string, hits []index.SearchHit) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d hits for %q (doc_slug | path | name | type):\n", len(hits), query)
	for _, h := range hits {
		fmt.Fprintf(&b, "%s | %s | %s | %s\n", h.DocSlug, h.Path, oneLine(h.Name), h.Type)
	}
	return b.String()
}

func formatPageHits(query string, hits []index.PageHit) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d page hits for %q (doc_slug | path | snippet):\n", len(hits), query)
	for _, h := range hits {
		fmt.Fprintf(&b, "%s | %s | %s\n", h.DocSlug, h.Path, oneLine(h.Snippet))
	}
	return b.String()
}

// oneLine collapses whitespace runs (including newlines) into single spaces.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// resolveSlug turns the doc_slug an agent passed into a known slug: a doc
// name such as "postgres" means its newest version. An empty slug stays
// empty. An unknown one is an error naming the closest docs.
func (d Deps) resolveSlug(ctx context.Context, slug string) (string, error) {
	if slug == "" {
		return "", nil
	}
	got, cands, err := d.Index.ResolveDocSlug(ctx, slug)
	if err != nil || got != "" {
		return got, err
	}
	if len(cands) > 0 {
		return "", fmt.Errorf("unknown doc_slug %q; did you mean: %s? "+
			"Or omit doc_slug to search all documentation", slug, strings.Join(cands, ", "))
	}
	return "", fmt.Errorf("unknown doc_slug %q; find the slug with list_docs(name) "+
		"or omit doc_slug to search all documentation", slug)
}

// textResult builds a CallToolResult with a single TextContent block — the
// human-readable representation that MCP clients show to the user. The
// structured fields go into the typed output parameter.
func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: s}},
	}
}
