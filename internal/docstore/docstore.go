// Package docstore provides read-only access to the DevDocs data layout.
//
// Layout on disk:
//
//	<root>/docs.json                  - list of all documents
//	<root>/<slug>/meta.json           - per-doc metadata (same shape as docs.json entry)
//	<root>/<slug>/index.json          - {entries: [...], types: [...]}
//	<root>/<slug>/db.zip              - one deflated entry per page: path -> html fragment
//
// db.zip is marvindocs' replacement for devdocs' db.json (see
// parsers/devdocsfmt.DBWriter and `marvindocs pack`).
package docstore

import (
	"archive/zip"
	"compress/flate"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	// dbSpans caches, per slug, where each page's data sits in db.zip.
	// Built on first access from the central directory; Page() then
	// reads and inflates a single entry, so a 270 MiB dump costs a few
	// hundred KiB of RSS instead of the whole decoded map.
	dbMu    sync.Mutex
	dbSpans map[string]map[string]span
	// scanMu serialises first-time scans so concurrent misses for the
	// same slug (get_docs reads pages in parallel) read the directory once.
	scanMu sync.Mutex
}

// dbName is the per-doc page archive.
const dbName = "db.zip"

// span locates one entry's data inside db.zip.
type span struct {
	off    int64  // data offset in the archive
	n      int64  // compressed size
	size   int64  // uncompressed size
	method uint16 // zip.Store or zip.Deflate
}

// Open loads docs.json. Per-doc index/db files are loaded lazily.
func Open(root string) (*Store, error) {
	s := &Store{
		root:     root,
		bySlug:   make(map[string]*Doc),
		indexMem: make(map[string]*Index),
		dbSpans:  make(map[string]map[string]span),
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

// ContentHash identifies a doc's content: the SHA-256 over every db.zip
// entry (path, CRC-32, size, as stored in the central directory) followed
// by the bytes of index.json. It never inflates a page, so it costs one
// central-directory read plus the index file, and it does not depend on
// the deflate implementation that packed the archive. A missing file is
// skipped, matching how the index treats docs that ship only one of them.
func (s *Store) ContentHash(slug string) (string, error) {
	h := sha256.New()
	zr, err := zip.OpenReader(filepath.Join(s.root, slug, dbName))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("hash %s for %s: %w", dbName, slug, err)
	}
	if err == nil {
		var rec [12]byte
		for _, f := range zr.File {
			io.WriteString(h, f.Name)
			binary.LittleEndian.PutUint32(rec[:4], f.CRC32)
			binary.LittleEndian.PutUint64(rec[4:], f.UncompressedSize64)
			h.Write(rec[:])
		}
		zr.Close()
	}
	f, err := os.Open(filepath.Join(s.root, slug, "index.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("hash index.json for %s: %w", slug, err)
	}
	if err == nil {
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return "", fmt.Errorf("hash index.json for %s: %w", slug, err)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

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
// The first call for a slug reads the db.zip directory once to record
// where each entry's data sits; later calls read and inflate just the
// requested page.
func (s *Store) Page(slug, path string) (string, error) {
	spans, err := s.spans(slug)
	if err != nil {
		return "", err
	}
	sp, ok := spans[path]
	if !ok {
		return "", fmt.Errorf("page not found: %s/%s", slug, path)
	}
	f, err := os.Open(filepath.Join(s.root, slug, dbName))
	if err != nil {
		return "", fmt.Errorf("open %s for %s: %w", dbName, slug, err)
	}
	defer f.Close()
	var r io.Reader = io.NewSectionReader(f, sp.off, sp.n)
	switch sp.method {
	case zip.Store:
	case zip.Deflate:
		fr := flate.NewReader(r)
		defer fr.Close()
		r = fr
	default:
		return "", fmt.Errorf("read page %s/%s: unsupported zip method %d", slug, path, sp.method)
	}
	buf := make([]byte, sp.size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", fmt.Errorf("read page %s/%s: %w", slug, path, err)
	}
	return string(buf), nil
}

// spans returns the cached entry table for slug, reading the db.zip
// directory on first use.
func (s *Store) spans(slug string) (map[string]span, error) {
	s.dbMu.Lock()
	spans, ok := s.dbSpans[slug]
	s.dbMu.Unlock()
	if ok {
		return spans, nil
	}
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	s.dbMu.Lock()
	spans, ok = s.dbSpans[slug] // filled by the scan we waited for
	s.dbMu.Unlock()
	if ok {
		return spans, nil
	}
	spans, err := s.scanSpans(slug)
	if err != nil {
		return nil, err
	}
	s.dbMu.Lock()
	s.dbSpans[slug] = spans
	s.dbMu.Unlock()
	return spans, nil
}

// scanSpans walks the central directory of db.zip and resolves each
// entry's data offset. The archive is closed afterwards: Page re-opens
// the file per read, so the store never holds a descriptor per doc.
func (s *Store) scanSpans(slug string) (map[string]span, error) {
	if _, ok := s.bySlug[slug]; !ok {
		return nil, fmt.Errorf("unknown slug: %s", slug)
	}
	zr, err := zip.OpenReader(filepath.Join(s.root, slug, dbName))
	if err != nil {
		return nil, fmt.Errorf("open %s for %s: %w", dbName, slug, err)
	}
	defer zr.Close()
	out := make(map[string]span, len(zr.File))
	for _, f := range zr.File {
		off, err := f.DataOffset()
		if err != nil {
			return nil, fmt.Errorf("scan %s for %s: entry %s: %w", dbName, slug, f.Name, err)
		}
		out[f.Name] = span{
			off:    off,
			n:      int64(f.CompressedSize64),
			size:   int64(f.UncompressedSize64),
			method: f.Method,
		}
	}
	return out, nil
}

// StreamPages calls fn for every page in db.zip in archive order without
// caching anything in the store. Intended for the indexer: peak memory is
// O(largest single page) instead of O(whole dump).
func (s *Store) StreamPages(slug string, fn func(path, html string) error) error {
	if _, ok := s.bySlug[slug]; !ok {
		return fmt.Errorf("unknown slug: %s", slug)
	}
	zr, err := zip.OpenReader(filepath.Join(s.root, slug, dbName))
	if err != nil {
		return fmt.Errorf("open %s for %s: %w", dbName, slug, err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open page %s/%s: %w", slug, f.Name, err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return fmt.Errorf("read page %s/%s: %w", slug, f.Name, err)
		}
		if err := fn(f.Name, string(b)); err != nil {
			return err
		}
	}
	return nil
}

// EvictDB drops the cached offset table for the given slug (used by
// the indexer after a doc is re-synced so stale spans are not served).
func (s *Store) EvictDB(slug string) {
	s.dbMu.Lock()
	delete(s.dbSpans, slug)
	s.dbMu.Unlock()
}
