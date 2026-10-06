package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

// runPack converts devdocs' db.json ({path: html}) into db.zip, the
// format docstore reads, and removes db.json afterwards. sync-docs.sh
// runs it on every freshly extracted dump; `pack --all` migrates an
// existing data directory in place. Docs that already have no db.json
// are skipped, so the command is safe to repeat.
func runPack(args []string) {
	fs := flag.NewFlagSet("pack", flag.ExitOnError)
	docsPath := fs.String("docs-path", "data/public/docs", "Path to devdocs data")
	all := fs.Bool("all", false, "Pack every doc under --docs-path instead of the listed slugs")
	_ = fs.Parse(args)

	slugs := fs.Args()
	if *all {
		ents, err := os.ReadDir(*docsPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		for _, e := range ents {
			if e.IsDir() {
				slugs = append(slugs, e.Name())
			}
		}
		sort.Strings(slugs)
	}
	if len(slugs) == 0 {
		fmt.Fprintln(os.Stderr, "error: pack needs slugs or --all")
		os.Exit(2)
	}

	failed := 0
	for _, slug := range slugs {
		dir := filepath.Join(*docsPath, slug)
		start := time.Now()
		pages, raw, packed, err := packDoc(dir)
		switch {
		case errors.Is(err, os.ErrNotExist):
			continue // already packed (or never had a db.json)
		case err != nil:
			slog.Error("pack failed", "doc", slug, "err", err)
			failed++
		default:
			slog.Info("packed", "doc", slug, "pages", pages,
				"raw_mb", raw>>20, "zip_mb", packed>>20, "duration", time.Since(start).Round(time.Millisecond))
		}
	}
	if failed > 0 {
		os.Exit(1)
	}
}

// packDoc streams dir/db.json into dir/db.zip and deletes db.json once
// the archive is in place. Returns os.ErrNotExist when there is nothing
// to pack. Decoding is key-by-key so peak memory is one page, not the
// whole dump.
func packDoc(dir string) (pages int, raw, packed int64, err error) {
	src := filepath.Join(dir, "db.json")
	f, err := os.Open(src)
	if err != nil {
		return 0, 0, 0, err
	}
	defer f.Close()

	w, err := devdocsfmt.NewDBWriter(filepath.Join(dir, devdocsfmt.DBName))
	if err != nil {
		return 0, 0, 0, err
	}
	dec := json.NewDecoder(f)
	tok, err := dec.Token()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("decode db.json: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return 0, 0, 0, fmt.Errorf("decode db.json: expected object, got %v", tok)
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return 0, 0, 0, fmt.Errorf("decode db.json: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return 0, 0, 0, fmt.Errorf("decode db.json: non-string key %v", keyTok)
		}
		var html string
		if err := dec.Decode(&html); err != nil {
			return 0, 0, 0, fmt.Errorf("decode db.json/%s: %w", key, err)
		}
		if err := w.Add(key, html); err != nil {
			return 0, 0, 0, err
		}
		pages++
	}
	if err := w.Close(); err != nil {
		return 0, 0, 0, err
	}
	st, err := os.Stat(filepath.Join(dir, devdocsfmt.DBName))
	if err != nil {
		return 0, 0, 0, err
	}
	if err := os.Remove(src); err != nil {
		return 0, 0, 0, fmt.Errorf("remove db.json: %w", err)
	}
	return pages, w.Size(), st.Size(), nil
}
