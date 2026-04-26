// Command-line helpers for ad-hoc searching without starting a server.
//
//	marvindocs search QUERY [--doc SLUG] [--limit N] [--json]
//	marvindocs get SLUG PATH [--format markdown|html]
//	marvindocs list [--json]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/piligrim/marvindocs/internal/docstore"
	"github.com/piligrim/marvindocs/internal/index"
)

// runSearch implements `marvindocs search QUERY ...`.
func runSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	docSlug := fs.String("doc", "", "Restrict results to this doc slug (optional)")
	limit := fs.Int("limit", 20, "Maximum number of results (1-100)")
	asJSON := fs.Bool("json", false, "Print results as JSON instead of a table")
	body := fs.Bool("body", false, "Search page bodies (requires index built with --fulltext)")
	indexPath := fs.String("index-path", "marvindocs.db", "SQLite index file")
	// Split positional query tokens from flags so users can mix them
	// freely: `search jsonb --doc postgresql~17` works the same as
	// `search --doc postgresql~17 jsonb`. flag.Parse stops on the
	// first non-flag, so we collect non-flag args manually.
	var positional, flagArgs []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
			// `--key value` consumes the next arg too.
			if !strings.Contains(a, "=") && i+1 < len(args) {
				next := args[i+1]
				// Treat as a flag value unless the next token also
				// starts with "-" (then it's a separate flag).
				if !strings.HasPrefix(next, "-") {
					flagArgs = append(flagArgs, next)
					i++
				}
			}
			continue
		}
		positional = append(positional, a)
	}
	_ = fs.Parse(flagArgs)

	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "usage: marvindocs search QUERY [--doc SLUG] [--limit N] [--json] [--body]")
		os.Exit(2)
	}
	query := strings.Join(positional, " ")

	idx, err := openIndexForRead(*indexPath)
	if err != nil {
		fatal(err)
	}
	defer idx.Close()

	if *body {
		hits, err := idx.SearchPages(context.Background(), query, *docSlug, *limit)
		if err != nil {
			fatal(err)
		}
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(hits)
			return
		}
		if len(hits) == 0 {
			fmt.Fprintln(os.Stderr, "no results (if the index was built without --fulltext, page-body search is empty)")
			return
		}
		for _, h := range hits {
			fmt.Printf("%-20s  %-40s  %s\n", h.DocSlug, h.Path, h.Snippet)
		}
		fmt.Fprintf(os.Stderr, "\n%d result(s)\n", len(hits))
		return
	}

	hits, err := idx.Search(context.Background(), query, *docSlug, *limit)
	if err != nil {
		fatal(err)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(hits)
		return
	}

	if len(hits) == 0 {
		fmt.Fprintln(os.Stderr, "no results")
		return
	}
	for _, h := range hits {
		fmt.Printf("%-20s  %-30s  %s\n", h.DocSlug, h.Name, h.Path)
	}
	fmt.Fprintf(os.Stderr, "\n%d result(s)\n", len(hits))
}

// runGet implements `marvindocs get SLUG PATH [--format markdown|html]`.
func runGet(args []string) {
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	format := fs.String("format", "markdown", "Output format: markdown or html")
	docsPath := fs.String("docs-path", "data/public/docs", "Path to the devdocs data directory")
	_ = fs.Parse(args)

	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: marvindocs get SLUG PATH [--format markdown|html]")
		os.Exit(2)
	}
	slug, path := fs.Arg(0), fs.Arg(1)

	store, err := docstore.Open(*docsPath)
	if err != nil {
		fatal(fmt.Errorf("open docstore: %w", err))
	}
	if store.Doc(slug) == nil {
		fatal(fmt.Errorf("unknown slug %q (use `marvindocs list` to see available)", slug))
	}

	html, err := store.Page(slug, path)
	if err != nil {
		fatal(err)
	}

	switch *format {
	case "html":
		fmt.Println(html)
	case "markdown", "md":
		md, err := docstore.ToMarkdown(html)
		if err != nil {
			fatal(fmt.Errorf("convert to markdown: %w", err))
		}
		fmt.Println(md)
	default:
		fatal(fmt.Errorf("unknown format %q (use markdown or html)", *format))
	}
}

// runList implements `marvindocs list` — prints indexed docs.
func runList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Print as JSON instead of a table")
	indexPath := fs.String("index-path", "marvindocs.db", "SQLite index file")
	_ = fs.Parse(args)

	idx, err := openIndexForRead(*indexPath)
	if err != nil {
		fatal(err)
	}
	defer idx.Close()

	docs, err := idx.ListDocs(context.Background())
	if err != nil {
		fatal(err)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(docs)
		return
	}

	for _, d := range docs {
		version := d.Version
		if version == "" {
			version = d.Release
		}
		fmt.Printf("%-28s  %-30s  %s\n", d.Slug, d.Name, version)
	}
	fmt.Fprintf(os.Stderr, "\n%d doc(s) indexed\n", len(docs))
}

// openIndexForRead opens the SQLite index or returns a friendly error.
func openIndexForRead(path string) (*index.Index, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("index file not found: %s (run `marvindocs index build` first)", path)
	}
	return index.Open(path)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
