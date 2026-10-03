package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/piligrim/marvindocs/internal/index"
)

// connect starts a server over a one-doc fixture index (slug "go",
// entry "Println") and returns a connected client session.
func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	idx, err := index.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	for _, q := range []string{
		`INSERT INTO docs(slug, name, type, version, release, mtime) VALUES('go','Go','lib','1','r1',0)`,
		`INSERT INTO entries(doc_slug, name, path, type) VALUES('go','Println','fmt/Println','Function')`,
		`INSERT INTO entries_fts(entries_fts) VALUES('rebuild')`,
	} {
		if _, err := idx.DB().Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	srv := New(Deps{Index: idx}, "test")
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
	want := "get_page,list_docs,search,search_pages"
	if strings.Join(got, ",") != want {
		t.Errorf("tools = %v, want %s", got, want)
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
	// Hits travel in structured content; the text block only has a summary.
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Println") {
		t.Errorf("result lacks entry name: %s", b)
	}
}
