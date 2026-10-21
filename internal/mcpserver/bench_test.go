package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/piligrim/marvindocs/internal/docstore"
	"github.com/piligrim/marvindocs/internal/embed"
	"github.com/piligrim/marvindocs/internal/index"
	"github.com/piligrim/marvindocs/internal/mdcache"
)

// benchCase is one retrieval question: the query as a developer would type
// it into get_docs, the doc it targets and the page paths that answer it.
// A result counts as a hit when one of Pages is the top ranked page (hit@1)
// or among the first three (hit@3). Kind is "lex" for keyword-style queries
// and "nl" for natural-language questions that share few words with the page.
type benchCase struct {
	Q     string   `json:"q"`
	Doc   string   `json:"doc"`
	Pages []string `json:"pages"`
	Kind  string   `json:"kind"`
}

func loadBenchCases(t *testing.T) []benchCase {
	t.Helper()
	b, err := os.ReadFile("testdata/bench_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []benchCase
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

// TestBenchCases checks that every case points at existing pages. It needs
// BENCH_DOCS (a docs directory with the benchmark docs) and runs without an
// index.
func TestBenchCases(t *testing.T) {
	dp := os.Getenv("BENCH_DOCS")
	if dp == "" {
		t.Skip("BENCH_DOCS not set")
	}
	store, err := docstore.Open(dp)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range loadBenchCases(t) {
		if c.Q == "" || len(c.Pages) == 0 || (c.Kind != "lex" && c.Kind != "nl") {
			t.Errorf("%s %q: need q, pages and kind lex|nl", c.Doc, c.Q)
			continue
		}
		for _, p := range c.Pages {
			if _, err := store.Page(c.Doc, p); err != nil {
				t.Errorf("%s %q: %v", c.Doc, c.Q, err)
			}
		}
	}
}

// TestBench measures get_docs retrieval quality on testdata/bench_cases.json.
// It is skipped unless BENCH_INDEX and BENCH_DOCS point at an index and the
// docs it was built from; BENCH_OLLAMA switches on the hybrid search and
// BENCH_WEIGHTS ("lex:title,lex:title,...") runs it once per weight pair:
//
//	BENCH_INDEX=marvindocs.db BENCH_DOCS=data/public/docs BENCH_OLLAMA=http://localhost:11434 \
//	BENCH_WEIGHTS=0.5:0,0.5:0.5 go test -tags sqlite_fts5 -run TestBench -v ./internal/mcpserver/
func TestBench(t *testing.T) {
	ip, dp := os.Getenv("BENCH_INDEX"), os.Getenv("BENCH_DOCS")
	if ip == "" || dp == "" {
		t.Skip("BENCH_INDEX/BENCH_DOCS not set")
	}
	store, err := docstore.Open(dp)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := index.Open(ip)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if os.Getenv("BENCH_SYNONYMS") == "off" {
		idx.SetSynonyms(nil)
	}
	d := Deps{Store: store, Index: idx, MDCache: mdcache.New(64)}
	if u := os.Getenv("BENCH_OLLAMA"); u != "" {
		d.Embedder = &cachingEmbedder{Embedder: embed.NewOllama(u, "bge-m3"), vecs: map[string][]float32{}}
	}
	cases := loadBenchCases(t)
	weights := os.Getenv("BENCH_WEIGHTS")
	if weights == "" {
		runBench(t, d, cases)
		return
	}
	for _, w := range strings.Split(weights, ",") {
		var lex, title float64
		if _, err := fmt.Sscanf(w, "%g:%g", &lex, &title); err != nil {
			t.Fatalf("BENCH_WEIGHTS %q: %v", w, err)
		}
		index.SetFusionWeights(lex, title)
		t.Logf("=== lexWeight=%g titleWeight=%g", lex, title)
		runBench(t, d, cases)
	}
}

// cachingEmbedder remembers each query vector so repeated bench runs in
// one process hit Ollama once per query.
type cachingEmbedder struct {
	embed.Embedder
	vecs map[string][]float32
}

func (c *cachingEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for n, s := range texts {
		v, ok := c.vecs[s]
		if !ok {
			vs, err := c.Embedder.Embed(ctx, []string{s})
			if err != nil {
				return nil, err
			}
			v = vs[0]
			c.vecs[s] = v
		}
		out[n] = v
	}
	return out, nil
}

func runBench(t *testing.T, d Deps, cases []benchCase) {
	type tally struct{ n, hit1, hit3 int }
	total := map[string]*tally{"all": {}, "lex": {}, "nl": {}}
	for _, c := range cases {
		frags, err := d.collectFragments(context.Background(), c.Q, c.Doc, 1<<30)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]bool{}
		for _, p := range c.Pages {
			want[p] = true
		}
		// Pages in the order of their best fragment.
		var order []string
		seen := map[string]bool{}
		for _, f := range frags {
			if !seen[f.Path] {
				seen[f.Path] = true
				order = append(order, f.Path)
			}
		}
		rank := 0
		for i, p := range order {
			if want[p] {
				rank = i + 1
				break
			}
		}
		for _, k := range []string{"all", c.Kind} {
			total[k].n++
			if rank == 1 {
				total[k].hit1++
			}
			if rank >= 1 && rank <= 3 {
				total[k].hit3++
			}
		}
		mark := "MISS"
		if rank > 0 {
			mark = fmt.Sprintf("@%d", rank)
		}
		top := ""
		if len(order) > 0 {
			top = order[0]
		}
		t.Logf("%-4s %-13s %-62q top=%s", mark, c.Doc, c.Q, top)
	}
	for _, k := range []string{"all", "lex", "nl"} {
		s := total[k]
		t.Logf("%-3s hit@1=%d/%d hit@3=%d/%d", k, s.hit1, s.n, s.hit3, s.n)
	}
}
