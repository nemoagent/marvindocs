package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// dataRoot writes a small devdocs tree with the docs the search tests
// rely on (angular, react, go), so they do not depend on which bundles
// are installed in data/public/docs.
func dataRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	docs := `[{"name":"Angular","slug":"angular","type":"angular","version":"","release":"20","mtime":1},
		{"name":"Go","slug":"go","type":"go","version":"","release":"1.25","mtime":1},
		{"name":"React","slug":"react","type":"react","version":"","release":"19","mtime":1}]`
	index := map[string]string{
		"angular": `{"entries":[{"name":"Component","path":"api/core/component","type":"Decorators"},
			{"name":"NgModule","path":"api/core/ngmodule","type":"Decorators"}],
			"types":[{"name":"Decorators","count":2,"slug":"decorators"}]}`,
		"go": `{"entries":[{"name":"context","path":"context/index","type":"context"},
			{"name":"context.Background","path":"context/index#Background","type":"context"},
			{"name":"context.WithCancel","path":"context/index#WithCancel","type":"context"}],
			"types":[{"name":"context","count":3,"slug":"context"}]}`,
		"react": `{"entries":[{"name":"useState","path":"reference/react/usestate","type":"Hooks"},
			{"name":"useEffect","path":"reference/react/useeffect","type":"Hooks"}],
			"types":[{"name":"Hooks","count":2,"slug":"hooks"}]}`,
	}
	pages := map[string]map[string]string{
		"angular": {
			"index":              "<h1>Angular</h1><p>Angular is a web framework for building component based applications.</p>",
			"api/core/component": "<h1>Component</h1><p>Decorator that marks a class as an Angular component.</p>",
			"api/core/ngmodule":  "<h1>NgModule</h1><p>Decorator that marks a class as an NgModule.</p>",
		},
		"go": {
			"context/index": "<h1>Package context</h1><p>WithCancel returns a derived context that is canceled when cancel is called.</p>",
		},
		"react": {
			"reference/react/usestate":  "<h1>useState</h1><p>useState is a React Hook that adds a state variable to a component.</p>",
			"reference/react/useeffect": "<h1>useEffect</h1><p>useEffect synchronizes a component with an external system.</p>",
		},
	}
	if err := os.WriteFile(filepath.Join(root, "docs.json"), []byte(docs), 0o644); err != nil {
		t.Fatal(err)
	}
	for slug, body := range index {
		dir := filepath.Join(root, slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		writeDB(t, dir, pages[slug])
	}
	return root
}

func TestOpenAndReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	idx, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	if err := idx.Reset(); err != nil {
		t.Fatal(err)
	}
	st, err := idx.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Docs != 0 || st.Entries != 0 {
		t.Fatalf("expected empty index, got %+v", st)
	}
}

func TestBuildPopulatesDocsAndEntries(t *testing.T) {
	idx := sharedIndex(t)

	got, err := idx.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Docs == 0 || got.Entries == 0 {
		t.Fatalf("expected non-empty stats, got %+v", got)
	}

	// Sanity: the angular doc should have been recorded.
	var name string
	row := idx.DB().QueryRow(`SELECT name FROM docs WHERE slug = 'angular'`)
	if err := row.Scan(&name); err != nil {
		t.Fatalf("angular not found in docs: %v", err)
	}
	if name == "" {
		t.Fatal("empty name for angular")
	}

	// Sanity: FTS should find at least one entry for a common term.
	var hits int
	row = idx.DB().QueryRow(`SELECT COUNT(*) FROM entries_fts WHERE entries_fts MATCH 'component'`)
	if err := row.Scan(&hits); err != nil {
		t.Fatal(err)
	}
	if hits == 0 {
		t.Fatal("expected FTS hits for 'component'")
	}
}
