package index

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/piligrim/marvindocs/internal/docstore"
)

// sharedIdx is built once per `go test` run and reused by all search tests
// to avoid paying the 2+ minute indexing cost N times.
var (
	sharedIdxOnce sync.Once
	sharedIdxPath string
	sharedIdxErr  error
)

func sharedIndex(t *testing.T) *Index {
	t.Helper()
	root := dataRoot(t) // may Skip()
	sharedIdxOnce.Do(func() {
		store, err := docstore.Open(root)
		if err != nil {
			sharedIdxErr = err
			return
		}
		dir, err := os.MkdirTemp("", "marvindocs-idx-*")
		if err != nil {
			sharedIdxErr = err
			return
		}
		sharedIdxPath = filepath.Join(dir, "shared.db")
		idx, err := Open(sharedIdxPath)
		if err != nil {
			sharedIdxErr = err
			return
		}
		defer idx.Close()
		if _, err := idx.Build(context.Background(), store); err != nil {
			sharedIdxErr = err
			return
		}
	})
	if sharedIdxErr != nil {
		t.Fatal(sharedIdxErr)
	}
	// Re-open a fresh handle for this test; cheap.
	idx, err := Open(sharedIdxPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	return idx
}

func TestSearchBasic(t *testing.T) {
	idx := sharedIndex(t)
	hits, err := idx.Search(context.Background(), "useState", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected hits for useState")
	}
	if len(hits) > 5 {
		t.Fatalf("limit not respected: got %d", len(hits))
	}
	for _, h := range hits {
		if h.DocSlug == "" || h.Name == "" || h.Path == "" {
			t.Fatalf("empty field in hit: %+v", h)
		}
	}
}

func TestSearchScopedToDoc(t *testing.T) {
	idx := sharedIndex(t)
	hits, err := idx.Search(context.Background(), "component", "angular", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected angular hits for 'component'")
	}
	for _, h := range hits {
		if h.DocSlug != "angular" {
			t.Fatalf("expected only angular hits, got %s", h.DocSlug)
		}
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	idx := sharedIndex(t)
	hits, err := idx.Search(context.Background(), "   ", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if hits != nil {
		t.Fatalf("expected nil hits for empty query, got %v", hits)
	}
}

func TestSearchMalformedQueryDoesNotError(t *testing.T) {
	idx := sharedIndex(t)
	// Characters that would break raw FTS5 syntax.
	_, err := idx.Search(context.Background(), `"quoted" AND (foo`, "", 5)
	if err != nil {
		t.Fatalf("sanitizer should have handled bad input: %v", err)
	}
}

func TestListDocs(t *testing.T) {
	idx := sharedIndex(t)
	docs, err := idx.ListDocs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) < 100 {
		t.Fatalf("expected many docs, got %d", len(docs))
	}
	// ordered by slug
	for i := 1; i < len(docs); i++ {
		if docs[i-1].Slug > docs[i].Slug {
			t.Fatalf("docs not ordered: %s > %s", docs[i-1].Slug, docs[i].Slug)
		}
	}
}

// TestSearchSubstringFallback verifies that LIKE-based fallback finds
// names whose tokens don't start with the query (e.g. 'cancel' inside
// 'WithCancel'), which FTS5's prefix-only matcher misses.
func TestSearchSubstringFallback(t *testing.T) {
	idx := sharedIndex(t)
	hits, err := idx.Search(context.Background(), "context cancel", "go", 10)
	if err != nil {
		t.Fatal(err)
	}
	var foundWithCancel bool
	for _, h := range hits {
		if strings.Contains(h.Name, "WithCancel") {
			foundWithCancel = true
			break
		}
	}
	if !foundWithCancel {
		names := make([]string, 0, len(hits))
		for _, h := range hits {
			names = append(names, h.Name)
		}
		t.Fatalf("expected WithCancel in results, got %v", names)
	}
}

func TestEscapeLike(t *testing.T) {
	cases := map[string]string{
		``:       ``,
		`foo`:    `foo`,
		`a%b`:    `a\%b`,
		`a_b`:    `a\_b`,
		`a\b`:    `a\\b`,
		`50%_x`:  `50\%\_x`,
	}
	for in, want := range cases {
		got := escapeLike(in)
		if got != want {
			t.Errorf("escapeLike(%q) = %q, want %q", in, got, want)
		}
	}
}
