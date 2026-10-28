package index

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/piligrim/marvindocs/internal/docstore"
	"github.com/piligrim/marvindocs/internal/synonyms"
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
		``:      ``,
		`foo`:   `foo`,
		`a%b`:   `a\%b`,
		`a_b`:   `a\_b`,
		`a\b`:   `a\\b`,
		`50%_x`: `50\%\_x`,
	}
	for in, want := range cases {
		got := escapeLike(in)
		if got != want {
			t.Errorf("escapeLike(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSearch_AutoDocFilter(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	idx, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	// Insert docs
	docs := []struct{ slug, name string }{
		{"python~3.13", "Python"},
		{"go-std~1.25", "Go"},
		{"react", "React"},
		{"async", "Async"},
		{"javascript", "JavaScript"},
	}
	for _, d := range docs {
		_, err := idx.db.Exec(`INSERT INTO docs(slug, name, type, version, release, mtime) VALUES(?,?,'lib','','',0)`, d.slug, d.name)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Insert entries
	entries := []struct{ slug, name, path, typ string }{
		{"python~3.13", "requests.get", "requests/get", "Method"},
		{"python~3.13", "requests.post", "requests/post", "Method"},
		{"go-std~1.25", "context.Background", "context/Background", "Function"},
		{"go-std~1.25", "context.WithCancel", "context/WithCancel", "Function"},
		{"react", "useState", "hooks/useState", "Function"},
		{"react", "useEffect", "hooks/useEffect", "Function"},
		{"async", "eachSeries", "index#eachSeries", "Collections"},
		{"javascript", "async function", "statements/async_function", "Statements"},
	}
	for _, e := range entries {
		_, err := idx.db.Exec(`INSERT INTO entries(doc_slug, name, path, type) VALUES(?,?,?,?)`, e.slug, e.name, e.path, e.typ)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Rebuild FTS
	_, err = idx.db.Exec(`INSERT INTO entries_fts(entries_fts) VALUES('rebuild')`)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	t.Run("python request", func(t *testing.T) {
		hits, err := idx.Search(ctx, "python request", "", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) == 0 {
			t.Fatal("expected hits for 'python request'")
		}
		for _, h := range hits {
			if !strings.HasPrefix(h.DocSlug, "python") {
				t.Errorf("expected python doc, got %s", h.DocSlug)
			}
		}
	})

	t.Run("go context", func(t *testing.T) {
		hits, err := idx.Search(ctx, "go context", "", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) == 0 {
			t.Fatal("expected hits for 'go context'")
		}
		for _, h := range hits {
			if !strings.HasPrefix(h.DocSlug, "go") {
				t.Errorf("expected go doc, got %s", h.DocSlug)
			}
		}
	})

	t.Run("useState react", func(t *testing.T) {
		hits, err := idx.Search(ctx, "useState react", "", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) == 0 {
			t.Fatal("expected hits for 'useState react'")
		}
		for _, h := range hits {
			if h.DocSlug != "react" {
				t.Errorf("expected react doc, got %s", h.DocSlug)
			}
		}
	})

	// "async" is a doc slug, but here it is an ordinary word: the
	// filtered search finds nothing, so a global search must run.
	t.Run("doc name as ordinary word", func(t *testing.T) {
		hits, err := idx.Search(ctx, "async function", "", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) == 0 || hits[0].DocSlug != "javascript" {
			t.Fatalf("expected javascript hit for 'async function', got %+v", hits)
		}
	})

	t.Run("useState no prefix", func(t *testing.T) {
		hits, err := idx.Search(ctx, "useState", "", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) == 0 {
			t.Fatal("expected hits for 'useState'")
		}
	})
}

// substringFixture opens a tiny index with a single doc and a few entries
// for exercising the LIKE fallback without the shared (slow) index.
func substringFixture(t *testing.T) *Index {
	t.Helper()
	idx, err := Open(filepath.Join(t.TempDir(), "sub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.db.Exec(`INSERT INTO docs(slug, name, type, version, release, mtime) VALUES('php','PHP','lib','','',0)`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"array_map", "fooBar", "a_b", "axb"} {
		if _, err := idx.db.Exec(`INSERT INTO entries(doc_slug, name, path, type) VALUES('php',?,?,'Function')`, name, name); err != nil {
			t.Fatal(err)
		}
	}
	return idx
}

func names(hits []SearchHit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Name)
	}
	return out
}

func TestSearchSubstring_PrefixThenContains(t *testing.T) {
	idx := substringFixture(t)
	ctx := context.Background()

	hits, err := idx.searchSubstring(ctx, "arr", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(hits); len(got) != 1 || got[0] != "array_map" {
		t.Fatalf("prefix pass: got %v, want [array_map]", got)
	}

	// No name starts with "bar"; the second ('%bar%') pass must find fooBar.
	hits, err = idx.searchSubstring(ctx, "bar", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(hits); len(got) != 1 || got[0] != "fooBar" {
		t.Fatalf("contains pass: got %v, want [fooBar]", got)
	}
}

func TestSearchSubstring_ShortQueryNeedsDocFilter(t *testing.T) {
	idx := substringFixture(t)
	ctx := context.Background()

	// "xb" is below minSubstringQueryLen and prefix-matches nothing:
	// the global '%xb%' scan is skipped.
	hits, err := idx.searchSubstring(ctx, "xb", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("unfiltered short query: got %v, want none", names(hits))
	}

	// With a doc filter the scan is bounded, so the substring hit is returned.
	hits, err = idx.searchSubstring(ctx, "xb", []string{"php"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(hits); len(got) != 1 || got[0] != "axb" {
		t.Fatalf("filtered short query: got %v, want [axb]", got)
	}
}

func TestSearchSubstring_PrefixEscapesWildcards(t *testing.T) {
	idx := substringFixture(t)
	hits, err := idx.searchSubstring(context.Background(), "a_b", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(hits); len(got) != 1 || got[0] != "a_b" {
		t.Fatalf("got %v, want [a_b]", got)
	}
}

func TestSearchSubstring_PrefixUsesNameIndex(t *testing.T) {
	idx := substringFixture(t)
	rows, err := idx.db.Query(`EXPLAIN QUERY PLAN SELECT doc_slug FROM entries WHERE name LIKE ? ESCAPE '\' LIMIT 10`, "arr%")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), "idx_entries_name") {
		t.Fatalf("prefix LIKE does not use idx_entries_name:\n%s", plan.String())
	}
}

func TestNewestPerDoc(t *testing.T) {
	got := newestPerDoc([]string{
		"python~2.7", "python~3.13", "python~3.9",
		"postgresql~9.4", "postgresql~17", "postgresql~16",
		"node~18_lts", "node~20_lts", "node",
		"go", "react~18", "react",
	})
	want := []string{"go", "node", "postgresql~17", "python~3.13", "react"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("newestPerDoc = %v, want %v", got, want)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"postgresql~17", "postgresql~9.4", 1},
		{"python~3.9", "python~3.13", -1},
		{"python~3.13", "python~3.13", 0},
		{"rails~7.1", "rails~7", 1},
		{"node~20_lts", "node~18_lts", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestChunkFTSQueryPrefix(t *testing.T) {
	got := chunkFTSQuery(nil, `wait for a group of goroutines`)
	want := `"wait"* AND "for" AND "a" AND "group"* AND "of" AND "goroutines"*`
	if got != want {
		t.Errorf("chunkFTSQuery = %s, want %s", got, want)
	}
	if got := sanitizeFTSQuery(nil, `a go`); got != `"a"* AND "go"*` {
		t.Errorf("sanitizeFTSQuery = %s", got)
	}
}

func TestFTSQuerySynonyms(t *testing.T) {
	th := &synonyms.Thesaurus{}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "re.md"), []byte("# regular expression\nsynonyms:: regex, re\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := th.LoadDir(dir); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ got, want string }{
		{chunkFTSQuery(th, `regex flags`), `("regex"* OR "regular expression"* OR "re") AND "flags"*`},
		{chunkFTSQuery(th, `Regular Expression`), `("Regular Expression"* OR "regex"* OR "re")`},
		{anyWordFTSQuery(th, `re flags`), `"re" OR "regular expression" OR "regex" OR "flags"`},
		{snippetQuery(th, `re flags`), `re regular expression regex flags`},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got %s, want %s", c.got, c.want)
		}
	}
}

func TestSearchSynonyms(t *testing.T) {
	idx, err := Open(filepath.Join(t.TempDir(), "syn.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	for _, slug := range []string{"postgresql~17", "kubernetes"} {
		if _, err := idx.db.Exec(`INSERT INTO docs(slug, name, type, version, release, mtime) VALUES(?,?,'lib','','',0)`, slug, slug); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range [][2]string{{"postgresql~17", "jsonb_path_query"}, {"kubernetes", "kubernetes deployment"}} {
		if _, err := idx.db.Exec(`INSERT INTO entries(doc_slug, name, path, type) VALUES(?,?,?,'Function')`, e[0], e[1], e[1]); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()

	// "pg" names the postgresql doc through the builtin thesaurus.
	hits, err := idx.Search(ctx, "pg jsonb", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].DocSlug != "postgresql~17" {
		t.Errorf("pg jsonb: %v", hits)
	}

	// "k8s" expands to "kubernetes" in the FTS query.
	hits, err = idx.Search(ctx, "k8s deployment", "kubernetes", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Name != "kubernetes deployment" {
		t.Errorf("k8s deployment: %v", hits)
	}

	idx.SetSynonyms(nil)
	hits, err = idx.Search(ctx, "k8s deployment", "kubernetes", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("k8s deployment without synonyms: %v", hits)
	}
}

func TestResolveDocSlug(t *testing.T) {
	idx, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	th, err := synonyms.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	idx.SetSynonyms(th)
	for _, s := range []string{"postgresql~9.4", "postgresql~18", "python~3.13", "react", "react_native", "go"} {
		if _, err := idx.db.Exec(`INSERT INTO docs(slug, name, type, version, release, mtime) VALUES(?,?,'lib','','',0)`, s, s); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		in, want string
		cands    int
	}{
		{"postgresql~9.4", "postgresql~9.4", 0},
		{"postgres", "postgresql~18", 0},
		{"PostgreSQL", "postgresql~18", 0},
		{"python", "python~3.13", 0},
		{"react", "react", 0},
		{"reac", "", 2},
		{"nosuch", "", 0},
	}
	for _, c := range cases {
		got, cands, err := idx.ResolveDocSlug(context.Background(), c.in)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want || len(cands) != c.cands {
			t.Errorf("ResolveDocSlug(%q) = %q, %v; want %q with %d candidates", c.in, got, cands, c.want, c.cands)
		}
	}
}

func TestOldVersions(t *testing.T) {
	got := oldVersions(map[string]bool{"postgresql~9.4": true, "postgresql~18": true, "react": true})
	if len(got) != 1 || got[0] != "postgresql~9.4" {
		t.Errorf("oldVersions = %v", got)
	}
}
