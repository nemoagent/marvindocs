// Package web implements the HTTP browser UI for marvindocs.
//
// Routes:
//
//	GET  /                          list of indexed documents
//	GET  /search?q=...&doc=...      search results (FTS5)
//	GET  /docs/{slug}/              redirect to the doc's index page
//	GET  /docs/{slug}/{path...}     render a single page
//	GET  /static/{file}             embedded CSS
//
// Rendering is server-side with html/template. Page bodies are the raw
// HTML fragments from devdocs' db.json; we wrap them in a shell that
// provides navigation and styling.
package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"github.com/piligrim/marvindocs/internal/docstore"
	"github.com/piligrim/marvindocs/internal/index"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Deps is the set of dependencies the web UI needs.
type Deps struct {
	Store *docstore.Store
	Index *index.Index
}

// Handler returns an http.Handler serving the web UI.
func Handler(d Deps) (http.Handler, error) {
	tpl, err := template.New("").Funcs(template.FuncMap{
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
	}).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, err
	}

	s := &server{deps: d, tpl: tpl}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /search", s.handleSearch)
	mux.HandleFunc("GET /docs/", s.handleDocPath)
	return mux, nil
}

type server struct {
	deps Deps
	tpl  *template.Template
}

func (s *server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.ExecuteTemplate(w, name, data); err != nil {
		// Template errors after bytes were flushed can't be recovered.
		// Log would go here if we had a logger; for now, best-effort 500.
		http.Error(w, "template: "+err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// "/" is handled here too because "GET /" matches everything under it.
	// Only the bare "/" path should show the index.
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	docs, err := s.deps.Index.ListDocs(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "index.html", map[string]any{
		"Title": "marvindocs",
		"Docs":  docs,
	})
}

func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	doc := r.URL.Query().Get("doc")
	var hits []index.SearchHit
	if q != "" {
		var err error
		hits, err = s.deps.Index.Search(r.Context(), q, doc, 50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	s.render(w, "search.html", map[string]any{
		"Title":   "search: " + q,
		"Query":   q,
		"DocSlug": doc,
		"Hits":    hits,
	})
}

func (s *server) handleDocPath(w http.ResponseWriter, r *http.Request) {
	// Path shape: /docs/{slug}/{page-path...}
	rest := strings.TrimPrefix(r.URL.Path, "/docs/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	slug, pagePath, _ := strings.Cut(rest, "/")
	if slug == "" {
		http.NotFound(w, r)
		return
	}

	doc := s.deps.Store.Doc(slug)
	if doc == nil {
		http.NotFound(w, r)
		return
	}

	// /docs/{slug}/ or /docs/{slug} -> default to "index" page.
	if pagePath == "" {
		pagePath = "index"
	}

	html, err := s.deps.Store.Page(slug, pagePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Build a sidebar of entries for navigation. Cheap: index.json is
	// cached after the first load.
	idx, _ := s.deps.Store.Index(slug)
	var entries []docstore.Entry
	if idx != nil {
		entries = idx.Entries
	}

	s.render(w, "page.html", map[string]any{
		"Title":    doc.Name + ": " + pagePath,
		"Doc":      doc,
		"PagePath": pagePath,
		"Body":     html,
		"Entries":  entries,
	})
}
