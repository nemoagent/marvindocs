// Package docstore provides read-only access to the DevDocs data layout.
//
// Layout on disk:
//
//	<root>/docs.json                  - list of all documents
//	<root>/<slug>/meta.json           - per-doc metadata (same shape as docs.json entry)
//	<root>/<slug>/index.json          - {entries: [...], types: [...]}
//	<root>/<slug>/db.json             - {path: "<html fragment>", ...}
package docstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Doc is one entry from docs.json / meta.json.
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
	Alias       string            `json:"alias,omitempty"`
}

// Entry is one symbol/page inside a doc (from index.json).
type Entry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

// DocType is a category inside a doc (from index.json).
type DocType struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Slug  string `json:"slug"`
}

// Index is the parsed index.json for a single doc.
type Index struct {
	Entries []Entry   `json:"entries"`
	Types   []DocType `json:"types"`
}

// Store is the read-only data accessor.
type Store struct {
	root string

	docs     []Doc
	bySlug   map[string]*Doc
	indexMu  sync.Mutex
	indexMem map[string]*Index // cached per-slug index.json

	dbMu  sync.Mutex
	dbMem map[string]map[string]string // cached per-slug db.json (optional, lazy)
}

// Open loads docs.json. Per-doc index/db files are loaded lazily.
func Open(root string) (*Store, error) {
	s := &Store{
		root:     root,
		bySlug:   make(map[string]*Doc),
		indexMem: make(map[string]*Index),
		dbMem:    make(map[string]map[string]string),
	}
	f, err := os.Open(filepath.Join(root, "docs.json"))
	if err != nil {
		return nil, fmt.Errorf("open docs.json: %w", err)
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&s.docs); err != nil {
		return nil, fmt.Errorf("decode docs.json: %w", err)
	}
	for i := range s.docs {
		s.bySlug[s.docs[i].Slug] = &s.docs[i]
	}
	return s, nil
}

// Root returns the data root directory.
func (s *Store) Root() string { return s.root }

// Docs returns all documents (sorted as in docs.json).
func (s *Store) Docs() []Doc { return s.docs }

// Doc returns a document by slug, or nil if not found.
func (s *Store) Doc(slug string) *Doc { return s.bySlug[slug] }

// Index returns the parsed index.json for the given slug.
// The result is cached in memory.
func (s *Store) Index(slug string) (*Index, error) {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()
	if idx, ok := s.indexMem[slug]; ok {
		return idx, nil
	}
	if _, ok := s.bySlug[slug]; !ok {
		return nil, fmt.Errorf("unknown slug: %s", slug)
	}
	f, err := os.Open(filepath.Join(s.root, slug, "index.json"))
	if err != nil {
		return nil, fmt.Errorf("open index.json for %s: %w", slug, err)
	}
	defer f.Close()
	var idx Index
	if err := json.NewDecoder(f).Decode(&idx); err != nil {
		return nil, fmt.Errorf("decode index.json for %s: %w", slug, err)
	}
	s.indexMem[slug] = &idx
	return &idx, nil
}

// Page returns the raw HTML fragment for the given path inside a doc.
// The whole db.json is read once per doc and cached in memory (cheap for
// one-off requests, heavy if you load many docs — prefer StreamPages for
// indexing).
func (s *Store) Page(slug, path string) (string, error) {
	s.dbMu.Lock()
	db, ok := s.dbMem[slug]
	s.dbMu.Unlock()
	if !ok {
		var err error
		db, err = s.loadDB(slug)
		if err != nil {
			return "", err
		}
		s.dbMu.Lock()
		s.dbMem[slug] = db
		s.dbMu.Unlock()
	}
	html, ok := db[path]
	if !ok {
		return "", fmt.Errorf("page not found: %s/%s", slug, path)
	}
	return html, nil
}

func (s *Store) loadDB(slug string) (map[string]string, error) {
	if _, ok := s.bySlug[slug]; !ok {
		return nil, fmt.Errorf("unknown slug: %s", slug)
	}
	f, err := os.Open(filepath.Join(s.root, slug, "db.json"))
	if err != nil {
		return nil, fmt.Errorf("open db.json for %s: %w", slug, err)
	}
	defer f.Close()
	var db map[string]string
	if err := json.NewDecoder(f).Decode(&db); err != nil {
		return nil, fmt.Errorf("decode db.json for %s: %w", slug, err)
	}
	return db, nil
}

// StreamPages calls fn for every page in db.json without holding the whole
// file in the store cache. Intended for the indexer.
func (s *Store) StreamPages(slug string, fn func(path, html string) error) error {
	db, err := s.loadDB(slug)
	if err != nil {
		return err
	}
	for p, h := range db {
		if err := fn(p, h); err != nil {
			return err
		}
	}
	return nil
}

// EvictDB drops the cached db.json for the given slug (useful during indexing
// to keep RAM bounded).
func (s *Store) EvictDB(slug string) {
	s.dbMu.Lock()
	delete(s.dbMem, slug)
	s.dbMu.Unlock()
}
