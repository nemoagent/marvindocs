package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/piligrim/marvindocs/internal/docstore"
	"github.com/piligrim/marvindocs/internal/index"
	"github.com/piligrim/marvindocs/internal/mdcache"
	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

// fixtureStore writes a one-doc devdocs tree (slug "go", entry
// "Println") with two pages.
func fixtureStore(t *testing.T) *docstore.Store {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"docs.json":     `[{"name":"Go","slug":"go","type":"lib","version":"1","release":"r1","mtime":0}]`,
		"go/index.json": `{"entries":[{"name":"Println","path":"fmt/Println","type":"Function"}],"types":[]}`,
	}
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w, err := devdocsfmt.NewDBWriter(filepath.Join(root, "go", devdocsfmt.DBName))
	if err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{
		"fmt/Println": "<h1>Println</h1><p>Println writes to standard output.</p>" +
			"<h2>Formatting</h2><p>Spaces are always added between operands.</p>",
		"os/exit": "<h1>Exit</h1><p>Exit causes the program to exit with the given code.</p>",
	}
	for path, html := range pages {
		if err := w.Add(path, html); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := docstore.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// connect builds the index from fixtureStore, starts a server over it
// and returns a connected client session.
func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	store := fixtureStore(t)
	idx, err := index.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Build(context.Background(), store); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	srv := New(Deps{Store: store, Index: idx, MDCache: mdcache.New(8)}, "test")
	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestListTools(t *testing.T) {
	cs := connect(t)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, tool := range res.Tools {
		got = append(got, tool.Name)
	}
	sort.Strings(got)
	want := "get_docs,get_page,list_docs,search,search_pages"
	if strings.Join(got, ",") != want {
		t.Errorf("tools = %v, want %s", got, want)
	}
}

func TestCallListDocsText(t *testing.T) {
	cs := connect(t)
	call := func(args map[string]any) string {
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_docs", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		return res.Content[0].(*mcp.TextContent).Text
	}
	if text := call(map[string]any{"name": "GO"}); !strings.Contains(text, "go | Go | 1") {
		t.Errorf("text block lacks doc: %q", text)
	}
	if text := call(map[string]any{"name": "rust"}); !strings.Contains(text, "No documentation set") {
		t.Errorf("unexpected match: %q", text)
	}
	if text := call(nil); !strings.Contains(text, "1 documentation sets indexed") || strings.Contains(text, "go | Go") {
		t.Errorf("no-name call should only summarize: %q", text)
	}
}

func TestCallSearchPagesText(t *testing.T) {
	cs := connect(t)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search_pages",
		Arguments: map[string]any{"query": "operands spaces"},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "go | fmt/Println | ") || !strings.Contains(text, "Spaces") {
		t.Errorf("text block lacks page hit: %q", text)
	}
}

func TestCallSearch(t *testing.T) {
	cs := connect(t)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search",
		Arguments: map[string]any{"query": "Println"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool returned error: %+v", res.Content)
	}
	// The text block must carry the hits too: clients may ignore
	// structured content.
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "go | fmt/Println | Println | Function") {
		t.Errorf("text block lacks hit: %q", text)
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Println") {
		t.Errorf("result lacks entry name: %s", b)
	}
}

// TestCallSearchPageFallback: a query that matches no entry name but
// occurs in a page body must return that page as a type "page" hit.
func TestCallSearchPageFallback(t *testing.T) {
	cs := connect(t)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search",
		Arguments: map[string]any{"query": "operands spaces"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool returned error: %+v", res.Content)
	}
	var out searchOutput
	b, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Hits) != 1 {
		t.Fatalf("hits = %d, want 1: %s", len(out.Hits), b)
	}
	h := out.Hits[0]
	if h.Type != "page" || h.Path != "fmt/Println" || !strings.Contains(h.Name, "[Spaces]") {
		t.Errorf("unexpected fallback hit: %+v", h)
	}
}

func TestCallGetDocs(t *testing.T) {
	cs := connect(t)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_docs",
		Arguments: map[string]any{"topic": "operands spaces", "tokens": 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool returned error: %+v", res.Content)
	}
	var out getDocsOutput
	b, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Fragments) == 0 {
		t.Fatalf("no fragments: %s", b)
	}
	// The "Formatting" section mentions both terms and must come first
	// (the short fixture page is a single chunk); the unrelated os/exit
	// page must not appear at all.
	if f := out.Fragments[0]; f.Path != "fmt/Println" || !strings.Contains(f.Content, "## Formatting") {
		t.Errorf("top fragment = %+v", f)
	}
	for _, f := range out.Fragments {
		if f.Path == "os/exit" {
			t.Errorf("unrelated page leaked into fragments: %+v", f)
		}
	}
	if out.Tokens > 100 {
		t.Errorf("tokens = %d, over budget", out.Tokens)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.HasPrefix(text, "Source: go/fmt/Println") {
		t.Errorf("text block = %q", text)
	}
}

func TestCallGetDocsResolvesSlug(t *testing.T) {
	cs := connect(t)
	for slug, wantErr := range map[string]bool{"Go": false, "nosuch": true} {
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "get_docs",
			Arguments: map[string]any{"topic": "operands spaces", "doc_slug": slug},
		})
		if err != nil {
			t.Fatal(err)
		}
		text := res.Content[0].(*mcp.TextContent).Text
		if res.IsError != wantErr {
			t.Errorf("doc_slug %q: IsError = %v, text %q", slug, res.IsError, text)
		}
		if wantErr && !strings.Contains(text, "unknown doc_slug") {
			t.Errorf("doc_slug %q: text %q", slug, text)
		}
	}
}
