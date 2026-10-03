// Package mcpserver exposes the marvindocs data to MCP clients (LLM agents).
//
// Three tools are registered:
//
//	list_docs  -> list all indexed documents
//	search     -> FTS5 search over entry names and paths
//	get_page   -> fetch a page as Markdown (default) or raw HTML
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
}

// New builds a configured mcp.Server with all marvindocs tools registered.
// Version identifies the server to clients.
func New(deps Deps, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "marvindocs",
		Version: version,
	}, nil)
	registerListDocs(s, deps)
	registerSearch(s, deps)
	registerSearchPages(s, deps)
	registerGetPage(s, deps)
	return s
}

// -------------------------------------------------------------------------
// list_docs
// -------------------------------------------------------------------------

type listDocsInput struct{}

type listDocsOutput struct {
	Docs []index.DocRow `json:"docs"`
}

func registerListDocs(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_docs",
		Description: "List all indexed documentation sets (name, slug, version).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ listDocsInput) (*mcp.CallToolResult, listDocsOutput, error) {
		docs, err := d.Index.ListDocs(ctx)
		if err != nil {
			return nil, listDocsOutput{}, err
		}
		return textResult(fmt.Sprintf("%d documents indexed", len(docs))),
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
			"Pass doc_slug to restrict results to a single documentation set.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, searchOutput, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}
		hits, err := d.Index.Search(ctx, in.Query, in.DocSlug, limit)
		if err != nil {
			return nil, searchOutput{}, err
		}
		if hits == nil {
			hits = []index.SearchHit{}
		}
		return textResult(fmt.Sprintf("%d hits for %q", len(hits), in.Query)),
			searchOutput{Hits: hits}, nil
	})
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
		hits, err := d.Index.SearchPages(ctx, d.Store, in.Query, in.DocSlug, limit)
		if err != nil {
			return nil, searchPagesOutput{}, err
		}
		if hits == nil {
			hits = []index.PageHit{}
		}
		return textResult(fmt.Sprintf("%d page hits for %q", len(hits), in.Query)),
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
			doc := d.Store.Doc(in.DocSlug)
			var release string
			if doc != nil {
				release = doc.Release
			}
			key := mdcache.Key(in.DocSlug, in.Path, release)
			if md, ok := d.MDCache.Get(key); ok {
				out.Content = md
			} else {
				md, err := docstore.ToMarkdown(html)
				if err != nil {
					return nil, getPageOutput{}, fmt.Errorf("convert to markdown: %w", err)
				}
				d.MDCache.Add(key, md)
				out.Content = md
			}
		default:
			return nil, getPageOutput{}, fmt.Errorf("unknown format %q (use 'markdown' or 'html')", in.Format)
		}
		return textResult(out.Content), out, nil
	})
}

// -------------------------------------------------------------------------
// helpers
// -------------------------------------------------------------------------

// textResult builds a CallToolResult with a single TextContent block — the
// human-readable representation that MCP clients show to the user. The
// structured fields go into the typed output parameter.
func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: s}},
	}
}
