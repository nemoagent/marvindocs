// Package devdocsfmt is the shared on-disk format produced by every
// marvindocs parser.
//
// The format is the same one DevDocs uses, so a parsed doc drops into
// data/public/docs/<slug>/ alongside docs scraped by upstream
// devdocs and is picked up by `marvindocs index build` without any
// special-casing.
//
// A parser populates a Writer with metadata, entries and pages, then
// calls Flush(outputDir). Flush emits three files:
//
//	<outputDir>/meta.json   - flat object describing the doc
//	<outputDir>/index.json  - {entries: [...], types: [...]}
//	<outputDir>/db.json     - {<path>: "<html fragment>", ...}
//
// Plus a .last-modified stamp so scripts/sync-docs.sh sees the doc
// as locally-managed and skips remote checks.
package devdocsfmt

// Doc is the metadata for a single documentation set. Fields mirror
// the keys DevDocs uses in its meta.json so existing tooling works
// unchanged.
type Doc struct {
	Name        string            `json:"name"`
	Slug        string            `json:"slug"`
	Type        string            `json:"type"`
	Links       map[string]string `json:"links,omitempty"`
	Version     string            `json:"version"`
	Release     string            `json:"release"`
	Mtime       int64             `json:"mtime"`
	DBSize      int64             `json:"db_size"`
	Attribution string            `json:"attribution,omitempty"`
}

// Entry is one searchable item inside a doc. Path is the
// db.json key (without trailing .html) plus optional "#anchor".
type Entry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

// DocType groups entries in the sidebar (e.g. "Hooks", "Commands").
// Count is filled in automatically by Flush.
type DocType struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Slug  string `json:"slug"`
}

// Page is one HTML fragment keyed by its on-disk path. The HTML
// must be a body fragment (no <html>/<head>); marvindocs wraps it
// in its own shell.
type Page struct {
	Path string
	HTML string
}
