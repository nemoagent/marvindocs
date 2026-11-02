package docstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

// dataRoot writes a small devdocs tree (angular, go) into a temp dir, so
// the tests do not depend on which bundles are installed in data/.
func dataRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	docs := `[{"name":"Angular","slug":"angular","type":"angular","release":"20","mtime":1},
		{"name":"Go","slug":"go","type":"go","release":"1.25","mtime":1}]`
	if err := os.WriteFile(filepath.Join(root, "docs.json"), []byte(docs), 0o644); err != nil {
		t.Fatal(err)
	}
	write := func(slug, index string, pages map[string]string) {
		dir := filepath.Join(root, slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte(index), 0o644); err != nil {
			t.Fatal(err)
		}
		w, err := devdocsfmt.NewDBWriter(filepath.Join(dir, devdocsfmt.DBName))
		if err != nil {
			t.Fatal(err)
		}
		for path, html := range pages {
			if err := w.Add(path, html); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	write("angular", `{"entries":[{"name":"Component","path":"api/core/component","type":"Decorators"}],
		"types":[{"name":"Decorators","count":1,"slug":"decorators"}]}`, map[string]string{
		"index":              "<h1>Angular</h1><p>Angular is a web framework for building component based applications. Angular is a web framework for building component based applications. Angular is a web framework for building component based applications. </p>",
		"api/core/component": "<h1>Component</h1><p>Decorator that marks a class as a component.</p>",
	})
	write("go", `{"entries":[{"name":"context","path":"context/index","type":"context"}],
		"types":[{"name":"context","count":1,"slug":"context"}]}`, map[string]string{
		"context/index": "<h1>Package context</h1>",
		"errors/index":  "<h1>Package errors</h1>",
	})
	return root
}

func TestOpenAndList(t *testing.T) {
	s, err := Open(dataRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Docs()) == 0 {
		t.Fatal("expected at least one doc")
	}
	if s.Doc("angular") == nil {
		t.Fatal("expected angular doc")
	}
}

func TestIndexAndPage(t *testing.T) {
	s, err := Open(dataRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	idx, err := s.Index("angular")
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Entries) == 0 || len(idx.Types) == 0 {
		t.Fatal("angular index should have entries and types")
	}
	html, err := s.Page("angular", "index")
	if err != nil {
		t.Fatal(err)
	}
	if len(html) < 100 {
		t.Fatalf("expected non-trivial page html, got %d bytes", len(html))
	}
}

func TestStreamPages(t *testing.T) {
	s, err := Open(dataRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	var count int
	var sample string
	if err := s.StreamPages("angular", func(path, html string) error {
		count++
		if path == "index" {
			sample = html
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("expected at least one streamed page")
	}
	if len(sample) < 100 {
		t.Fatalf("index page should be non-trivial, got %d bytes", len(sample))
	}
}

func TestPageMatchesStreamPages(t *testing.T) {
	s, err := Open(dataRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	slug := "go"
	const limit = 200
	full := map[string]string{}
	stop := errors.New("enough")
	err = s.StreamPages(slug, func(path, html string) error {
		full[path] = html
		if len(full) == limit {
			return stop
		}
		return nil
	})
	if err != nil && !errors.Is(err, stop) {
		t.Fatal(err)
	}
	for path, want := range full {
		got, err := s.Page(slug, path)
		if err != nil {
			t.Fatalf("Page(%s, %s): %v", slug, path, err)
		}
		if got != want {
			t.Fatalf("Page(%s, %s) differs from StreamPages", slug, path)
		}
	}
	if _, err := s.Page(slug, "no/such/page"); err == nil {
		t.Fatal("expected error for missing page")
	}
}
