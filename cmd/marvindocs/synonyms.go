// Preparing synonym files from the docs themselves.
//
//	marvindocs synonyms suggest [--docs-path DIR] [--doc SLUG]... [--min-docs N] [--min-count N] [--names-only] [--synonyms DIR] [--out FILE]
//	marvindocs synonyms import FILE --out DIR [--force]
//
// suggest writes a TSV table of candidates (concept, synonyms, count,
// docs); after review (delete rows, edit synonyms, add translations)
// import turns it into one *.md file per concept for --synonyms DIR.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/piligrim/marvindocs/internal/docstore"
	"github.com/piligrim/marvindocs/internal/synonyms"
)

func runSynonyms(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: marvindocs synonyms (suggest|import) ...")
		os.Exit(2)
	}
	switch args[0] {
	case "suggest":
		runSynonymsSuggest(args[1:])
	case "import":
		runSynonymsImport(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: synonyms %s\n", args[0])
		os.Exit(2)
	}
}

func runSynonymsSuggest(args []string) {
	fs := flag.NewFlagSet("synonyms suggest", flag.ExitOnError)
	docsPath := fs.String("docs-path", "data/public/docs", "Path to devdocs data")
	var only []string
	fs.Func("doc", "Doc slug to scan; may be repeated (default: newest version of every doc)", func(v string) error {
		only = append(only, v)
		return nil
	})
	minDocs := fs.Int("min-docs", 2, "Keep pairs found in at least N docs (versions of a doc count once)")
	minCount := fs.Int("min-count", 3, "Keep pairs found at least N times")
	namesOnly := fs.Bool("names-only", false, "Scan entry names only, not page text (seconds instead of minutes)")
	synDir := synonymsFlag(fs)
	out := fs.String("out", "", "Write the table to FILE instead of stdout")
	_ = fs.Parse(args)

	store, err := docstore.Open(*docsPath)
	if err != nil {
		fatal(err)
	}
	slugs := only
	if len(slugs) == 0 {
		slugs = newestVersions(store.Docs())
	}
	known, err := knownSynonyms(*synDir)
	if err != nil {
		fatal(err)
	}

	start := time.Now()
	var m synonyms.Miner
	jobs := make(chan string)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for slug := range jobs {
				if err := mineDoc(&m, store, slug, *namesOnly); err != nil {
					slog.Warn("skip doc", "doc", slug, "err", err)
				}
			}
		}()
	}
	for _, s := range slugs {
		jobs <- s
	}
	close(jobs)
	wg.Wait()

	cands := m.Candidates(*minDocs, *minCount, known)
	w := io.Writer(os.Stdout)
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			fatal(err)
		}
		defer f.Close()
		w = f
	}
	bw := bufio.NewWriter(w)
	fmt.Fprintln(bw, "# concept\tsynonyms\tcount\tdocs\texamples")
	for _, c := range cands {
		ex := c.Docs[:min(len(c.Docs), 5)]
		fmt.Fprintf(bw, "%s\t%s\t%d\t%d\t%s\n", c.Concept, strings.Join(c.Synonyms, ", "), c.Count, len(c.Docs), strings.Join(ex, ", "))
	}
	if err := bw.Flush(); err != nil {
		fatal(err)
	}
	slog.Info("synonym candidates", "docs", len(slugs), "candidates", len(cands), "duration", time.Since(start).Round(time.Second))
}

// newestVersions returns one slug per doc (the part before "~"), the
// most recently scraped one: versions repeat the same text.
func newestVersions(docs []docstore.Doc) []string {
	best := make(map[string]docstore.Doc)
	var order []string
	for _, d := range docs {
		base, _, _ := strings.Cut(d.Slug, "~")
		cur, ok := best[base]
		if !ok {
			order = append(order, base)
		}
		if !ok || d.Mtime > cur.Mtime {
			best[base] = d
		}
	}
	slugs := make([]string, len(order))
	for k, base := range order {
		slugs[k] = best[base].Slug
	}
	return slugs
}

// knownSynonyms loads what --synonyms would use, so that suggest skips
// terms that already have synonyms.
func knownSynonyms(dir string) (*synonyms.Thesaurus, error) {
	if dir == "off" {
		return nil, nil
	}
	th, err := synonyms.Builtin()
	if err != nil || dir == "" {
		return th, err
	}
	return th, th.LoadDir(dir)
}

// mineDoc feeds the entry names and, unless namesOnly, the page text of
// one doc to m, under the doc's name without version.
func mineDoc(m *synonyms.Miner, store *docstore.Store, slug string, namesOnly bool) error {
	base, _, _ := strings.Cut(slug, "~")
	idx, err := store.Index(slug)
	if err != nil {
		return err
	}
	for _, e := range idx.Entries {
		m.Add(base, e.Name)
	}
	if namesOnly {
		return nil
	}
	return store.StreamPages(slug, func(_, html string) error {
		m.Add(base, docstore.ToPlainText(html))
		return nil
	})
}

func runSynonymsImport(args []string) {
	fs := flag.NewFlagSet("synonyms import", flag.ExitOnError)
	out := fs.String("out", "", "Directory to write *.md synonym files to (required)")
	force := fs.Bool("force", false, "Overwrite existing files")
	// Accept the table path before or after the flags.
	var file string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		file, args = args[0], args[1:]
	}
	_ = fs.Parse(args)
	if file == "" && fs.NArg() > 0 {
		file = fs.Arg(0)
	}
	if file == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: marvindocs synonyms import FILE --out DIR [--force]")
		os.Exit(2)
	}

	in := io.Reader(os.Stdin)
	if file != "-" {
		f, err := os.Open(file)
		if err != nil {
			fatal(err)
		}
		defer f.Close()
		in = f
	}
	concepts, order, err := readSynonymTable(in)
	if err != nil {
		fatal(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fatal(err)
	}
	written, skipped := 0, 0
	for _, c := range order {
		path := filepath.Join(*out, synonymFileName(c))
		if _, err := os.Stat(path); err == nil && !*force {
			skipped++
			continue
		}
		body := "# " + c + "\n\nsynonyms:: " + strings.Join(concepts[c], ", ") + "\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			fatal(err)
		}
		written++
	}
	slog.Info("synonym files", "dir", *out, "written", written, "skipped_existing", skipped)
}

// readSynonymTable parses the suggest table: concept, then a comma list
// of synonyms; other columns, blank lines and "#" lines are ignored.
// Rows of the same concept merge. order keeps the first appearance.
func readSynonymTable(r io.Reader) (concepts map[string][]string, order []string, err error) {
	concepts = make(map[string][]string)
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		concept := strings.TrimSpace(cols[0])
		if len(cols) < 2 || concept == "" {
			return nil, nil, fmt.Errorf("line %d: want concept<TAB>synonyms", n)
		}
		if _, ok := concepts[concept]; !ok {
			order = append(order, concept)
		}
		for _, s := range strings.Split(cols[1], ",") {
			if s = strings.TrimSpace(s); s != "" && !slices.Contains(concepts[concept], s) {
				concepts[concept] = append(concepts[concept], s)
			}
		}
	}
	return concepts, order, sc.Err()
}

// synonymFileName turns a concept into a file name: "server-side
// rendering" -> "server-side-rendering.md".
func synonymFileName(concept string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(concept) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-") + ".md"
}
