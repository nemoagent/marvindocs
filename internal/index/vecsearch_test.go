package index

import (
	"context"
	"hash/fnv"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/piligrim/marvindocs/internal/docstore"
)

// fakeEmbedder maps every text to a deterministic unit vector derived
// from its hash and remembers which texts it was asked for.
type fakeEmbedder struct {
	calls int
	seen  map[string][]float32
}

func (f *fakeEmbedder) Dim() int     { return 32 }
func (f *fakeEmbedder) Close() error { return nil }

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if f.seen == nil {
		f.seen = make(map[string][]float32)
	}
	out := make([][]float32, len(texts))
	for n, t := range texts {
		h := fnv.New64a()
		_, _ = h.Write([]byte(t))
		r := rand.New(rand.NewSource(int64(h.Sum64())))
		v := make([]float32, f.Dim())
		var sum float64
		for k := range v {
			v[k] = float32(r.NormFloat64())
			sum += float64(v[k]) * float64(v[k])
		}
		for k := range v {
			v[k] /= float32(math.Sqrt(sum))
		}
		out[n] = v
		f.seen[t] = v
		f.calls++
	}
	return out, nil
}

// vecFixture indexes a doc with three one-chunk pages.
func vecFixture(t *testing.T) (*Index, *docstore.Store) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"docs.json":        `[{"name":"Alpha","slug":"alpha","type":"lib","version":"1","release":"r1","mtime":1}]`,
		"alpha/index.json": `{"entries":[{"name":"Connect","path":"net/connect","type":"Function"}],"types":[]}`,
		"alpha/db.json": `{"net/connect":"<h1>Connect</h1><p>Opens a pooled connection to the server.</p>",
			"net/close":"<h1>Close</h1><p>Releases the connection back to the pool.</p>",
			"fs/read":"<h1>Read</h1><p>Reads bytes from a file descriptor.</p>"}`,
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
	store, err := docstore.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := Open(filepath.Join(t.TempDir(), "vec.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Build(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	return idx, store
}

func count(t *testing.T, idx *Index, table string) int {
	t.Helper()
	var n int
	if err := idx.DB().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEmbedDocAndSearchVec(t *testing.T) {
	idx, store := vecFixture(t)
	ctx := context.Background()
	e := &fakeEmbedder{}

	if has, err := idx.HasVectors(ctx, "alpha"); err != nil || has {
		t.Fatalf("HasVectors before embedding = %v, %v", has, err)
	}
	var lastDone, lastTotal int
	st, err := idx.EmbedAll(ctx, store, nil, e, 2, func(done, total int) { lastDone, lastTotal = done, total })
	if err != nil {
		t.Fatal(err)
	}
	chunks := count(t, idx, "chunks")
	if chunks != 3 || st.Chunks != chunks || st.Pages != 3 || count(t, idx, "chunk_vecs") != chunks {
		t.Fatalf("stats = %+v, chunks = %d, vecs = %d", st, chunks, count(t, idx, "chunk_vecs"))
	}
	if lastDone != 3 || lastTotal != 3 {
		t.Fatalf("progress = %d/%d, want 3/3", lastDone, lastTotal)
	}
	if has, err := idx.HasVectors(ctx, "alpha"); err != nil || !has {
		t.Fatalf("HasVectors after embedding = %v, %v", has, err)
	}

	// A second run finds nothing pending.
	calls := e.calls
	st, err = idx.EmbedDoc(ctx, store, "alpha", e, 2, nil)
	if err != nil || st.Chunks != 0 || e.calls != calls {
		t.Fatalf("second run: %+v, %v, embed calls %d -> %d", st, err, calls, e.calls)
	}

	// Query with the exact vector of the "pooled" chunk.
	var q []float32
	for text, v := range e.seen {
		if strings.Contains(text, "pooled") {
			if !strings.HasPrefix(text, "alpha > Connect\n") {
				t.Fatalf("unexpected embed text %q", text)
			}
			q = v
		}
	}
	if q == nil {
		t.Fatal("no text with 'pooled' was embedded")
	}
	hits, err := idx.SearchChunksVec(ctx, q, "alpha", 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].Path != "net/connect" || hits[0].Score > -0.99 || hits[0].Score >= hits[1].Score {
		t.Fatalf("chunk hits = %+v", hits)
	}
	pageHits, err := idx.SearchPagesVec(ctx, store, q, "pooled", "alpha", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(pageHits) != 3 || pageHits[0].Path != "net/connect" || !strings.Contains(pageHits[0].Snippet, "[pooled]") {
		t.Fatalf("page hits = %+v", pageHits)
	}
	if _, err := idx.SearchChunksVec(ctx, q, "", 2, 1); err == nil {
		t.Fatal("expected an error without a doc slug")
	}

	// Hybrid: the vector points at Connect, the words at Read; Read is in
	// both lists and wins, every hit has a path.
	hits, err = idx.SearchChunksHybrid(ctx, q, "reads bytes descriptor", "alpha", 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 || hits[0].Path != "fs/read" || hits[1].Path != "net/connect" || hits[2].Path == "" {
		t.Fatalf("hybrid hits = %+v", hits)
	}

	// Deleting the doc as Sync does removes its vectors.
	tx, err := idx.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := deleteDocRows(ctx, tx, "alpha"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if n := count(t, idx, "chunk_vecs"); n != 0 {
		t.Fatalf("chunk_vecs rows after delete = %d, want 0", n)
	}
}

func TestTruncateUTF8(t *testing.T) {
	if got := truncateUTF8("aб", 2); got != "a" {
		t.Fatalf("truncateUTF8 = %q, want %q", got, "a")
	}
	if got := truncateUTF8("abc", 10); got != "abc" {
		t.Fatalf("truncateUTF8 = %q", got)
	}
}
