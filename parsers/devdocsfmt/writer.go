package devdocsfmt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Writer accumulates entries and pages for one doc, then writes the
// three devdocs files plus a stamp atomically.
//
// Typical usage:
//
//	w := devdocsfmt.NewWriter(devdocsfmt.Doc{Name: "Laravel", ...})
//	for each page in source:
//	    w.AddPage(path, html)
//	    for each symbol on page:
//	        w.AddEntry(name, path+"#"+anchor, sectionType)
//	if err := w.Flush("data/public/docs/laravel~12", false); err != nil {...}
type Writer struct {
	Doc     Doc
	pages   map[string]string // path -> html
	entries []Entry
}

// NewWriter creates an empty Writer with the given doc metadata.
// Mtime defaults to "now" if the caller leaves it as 0.
func NewWriter(d Doc) *Writer {
	if d.Mtime == 0 {
		d.Mtime = time.Now().Unix()
	}
	return &Writer{
		Doc:   d,
		pages: make(map[string]string),
	}
}

// AddPage stores the HTML fragment for path. Calling it twice with
// the same path overwrites the previous content.
func (w *Writer) AddPage(path, html string) {
	w.pages[path] = html
}

// AddEntry registers a searchable name. type is the doc-specific
// section ("Hooks", "Commands"); marvindocs uses it to group results.
func (w *Writer) AddEntry(name, path, typ string) {
	w.entries = append(w.entries, Entry{Name: name, Path: path, Type: typ})
}

// Flush writes meta.json, index.json, db.json and .last-modified to
// outputDir. The directory is created if missing. When force is
// false and the directory already contains any of the output files,
// Flush returns an error so callers don't silently overwrite work.
func (w *Writer) Flush(outputDir string, force bool) error {
	if len(w.pages) == 0 {
		return fmt.Errorf("devdocsfmt: no pages to write")
	}
	if w.Doc.Slug == "" || w.Doc.Name == "" {
		return fmt.Errorf("devdocsfmt: Doc.Name and Doc.Slug are required")
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outputDir, err)
	}
	if !force {
		for _, name := range []string{"meta.json", "index.json", "db.json"} {
			if _, err := os.Stat(filepath.Join(outputDir, name)); err == nil {
				return fmt.Errorf("%s already exists in %s (pass --force to overwrite)", name, outputDir)
			}
		}
	}

	dbBytes, err := json.Marshal(w.pages)
	if err != nil {
		return fmt.Errorf("marshal db.json: %w", err)
	}

	doc := w.Doc
	doc.DBSize = int64(len(dbBytes))
	if doc.Mtime == 0 {
		doc.Mtime = time.Now().Unix()
	}

	idx := struct {
		Entries []Entry   `json:"entries"`
		Types   []DocType `json:"types"`
	}{
		Entries: sortedEntries(w.entries),
		Types:   buildTypes(w.entries),
	}

	if err := writeJSON(filepath.Join(outputDir, "db.json"), dbBytes); err != nil {
		return err
	}
	if err := writeJSONIndent(filepath.Join(outputDir, "index.json"), idx); err != nil {
		return err
	}
	if err := writeJSONIndent(filepath.Join(outputDir, "meta.json"), doc); err != nil {
		return err
	}

	stamp := filepath.Join(outputDir, ".last-modified")
	if err := os.WriteFile(stamp, nil, 0o644); err != nil {
		return fmt.Errorf("write stamp: %w", err)
	}
	t := time.Unix(doc.Mtime, 0)
	if err := os.Chtimes(stamp, t, t); err != nil {
		return fmt.Errorf("touch stamp: %w", err)
	}
	return nil
}

// sortedEntries returns a stable copy ordered by (Type, Name) so
// generated index.json diffs cleanly across re-runs.
func sortedEntries(in []Entry) []Entry {
	out := append([]Entry(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// buildTypes derives the unique types and their counts from entries.
// Slug is the type name lowercased (matches DevDocs convention).
func buildTypes(entries []Entry) []DocType {
	counts := map[string]int{}
	for _, e := range entries {
		if e.Type == "" {
			continue
		}
		counts[e.Type]++
	}
	out := make([]DocType, 0, len(counts))
	for name, n := range counts {
		out = append(out, DocType{Name: name, Count: n, Slug: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func writeJSON(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func writeJSONIndent(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	return writeJSON(path, data)
}
