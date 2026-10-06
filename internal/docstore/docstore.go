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
	"bufio"
	"crypto/sha256"
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

	// dbSpans caches, per slug, where each page's JSON string sits in
	// db.json. Built on first access by one streaming pass; Page() then
	// reads a single value with ReadAt, so a 270 MiB dump costs a few
	// hundred KiB of RSS instead of the whole decoded map.
	dbMu    sync.Mutex
	dbSpans map[string]map[string]span
	// scanMu serialises first-time scans so concurrent misses for the
	// same slug (get_docs reads pages in parallel) scan db.json once.
	scanMu sync.Mutex
}

// span locates a JSON string value inside db.json.
type span struct {
	off int64
	n   int64
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

// ContentHash returns the hex SHA-256 of a doc's db.json followed by its
// index.json. A missing file is skipped, matching how the index treats
// docs that ship only one of them.
func (s *Store) ContentHash(slug string) (string, error) {
	h := sha256.New()
	for _, name := range []string{"db.json", "index.json"} {
		f, err := os.Open(filepath.Join(s.root, slug, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("hash %s for %s: %w", name, slug, err)
		}
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return "", fmt.Errorf("hash %s for %s: %w", name, slug, err)
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
// The first call for a slug scans db.json once to record value offsets;
// later calls read just the requested page from disk.
func (s *Store) Page(slug, path string) (string, error) {
	spans, err := s.spans(slug)
	if err != nil {
		return "", err
	}
	sp, ok := spans[path]
	if !ok {
		return "", fmt.Errorf("page not found: %s/%s", slug, path)
	}
	f, err := os.Open(filepath.Join(s.root, slug, "db.json"))
	if err != nil {
		return "", fmt.Errorf("open db.json for %s: %w", slug, err)
	}
	defer f.Close()
	buf := make([]byte, sp.n)
	if _, err := f.ReadAt(buf, sp.off); err != nil {
		return "", fmt.Errorf("read page %s/%s: %w", slug, path, err)
	}
	var html string
	if err := json.Unmarshal(buf, &html); err != nil {
		return "", fmt.Errorf("decode page %s/%s: %w", slug, path, err)
	}
	return html, nil
}

// spans returns the cached offset table for slug, scanning db.json on
// first use.
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

func (s *Store) scanSpans(slug string) (map[string]span, error) {
	if _, ok := s.bySlug[slug]; !ok {
		return nil, fmt.Errorf("unknown slug: %s", slug)
	}
	f, err := os.Open(filepath.Join(s.root, slug, "db.json"))
	if err != nil {
		return nil, fmt.Errorf("open db.json for %s: %w", slug, err)
	}
	defer f.Close()
	spans, err := scanSpans(bufio.NewReaderSize(f, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("scan db.json for %s: %w", slug, err)
	}
	return spans, nil
}

// scanSpans walks a db.json object ({"path": "<html>", ...}) and records
// the byte range of every string value without decoding it. It relies on
// the fixed shape of db.json: a flat object whose values are all strings.
// Anything else is reported as an error.
//
// Strings are skipped with ReadSlice so the hot loop runs at buffer
// speed (~700 MiB/s) instead of byte-by-byte; encoding/json's streaming
// decoder is roughly ten times slower on the largest dumps.
func scanSpans(br *bufio.Reader) (map[string]span, error) {
	var pos int64
	next := func() (byte, error) {
		b, err := br.ReadByte()
		pos++
		return b, err
	}
	skipWS := func() (byte, error) {
		for {
			b, err := next()
			if err != nil {
				return 0, err
			}
			if b != ' ' && b != '\n' && b != '\r' && b != '\t' {
				return b, nil
			}
		}
	}
	// skipString consumes a string whose opening quote was already read.
	// When keep is set, it returns the raw bytes including both quotes.
	skipString := func(keep bool) ([]byte, error) {
		var buf []byte
		if keep {
			buf = append(buf, '"')
		}
		carry := 0 // backslashes at the end of the previous chunk
		for {
			seg, err := br.ReadSlice('"')
			pos += int64(len(seg))
			if keep {
				buf = append(buf, seg...)
			}
			if errors.Is(err, bufio.ErrBufferFull) {
				carry = trailingBackslashes(seg, carry)
				continue
			}
			if err != nil {
				return nil, err
			}
			// An odd number of backslashes before the quote escapes it.
			if trailingBackslashes(seg[:len(seg)-1], carry)%2 == 0 {
				return buf, nil
			}
			carry = 0
		}
	}

	b, err := skipWS()
	if err != nil || b != '{' {
		return nil, errors.New("expected object")
	}
	out := make(map[string]span)
	for {
		b, err := skipWS()
		if err != nil {
			return nil, err
		}
		if b == '}' {
			return out, nil
		}
		if b != '"' {
			return nil, fmt.Errorf("expected key at offset %d", pos-1)
		}
		rawKey, err := skipString(true)
		if err != nil {
			return nil, err
		}
		var key string
		if err := json.Unmarshal(rawKey, &key); err != nil {
			return nil, err
		}
		if b, err = skipWS(); err != nil || b != ':' {
			return nil, fmt.Errorf("expected ':' after key %q", key)
		}
		if b, err = skipWS(); err != nil || b != '"' {
			return nil, fmt.Errorf("expected string value for key %q", key)
		}
		start := pos - 1
		if _, err = skipString(false); err != nil {
			return nil, err
		}
		out[key] = span{off: start, n: pos - start}
		if b, err = skipWS(); err != nil {
			return nil, err
		}
		if b == '}' {
			return out, nil
		}
		if b != ',' {
			return nil, fmt.Errorf("expected ',' after key %q", key)
		}
	}
}

// trailingBackslashes counts consecutive backslashes at the end of seg.
// If seg is all backslashes, carry (the count from the preceding chunk)
// is added so escapes split across chunks are still counted correctly.
func trailingBackslashes(seg []byte, carry int) int {
	n := 0
	for i := len(seg) - 1; i >= 0 && seg[i] == '\\'; i-- {
		n++
	}
	if n == len(seg) {
		n += carry
	}
	return n
}

// StreamPages calls fn for every page in db.json without holding the whole
// file in the store cache. Intended for the indexer.
//
// Decoding is streamed key-by-key, so peak memory is O(largest single
// page) instead of O(whole db.json). For multi-hundred-MB devdocs dumps
// this is the difference between a few MB and several GB of RSS.
func (s *Store) StreamPages(slug string, fn func(path, html string) error) error {
	if _, ok := s.bySlug[slug]; !ok {
		return fmt.Errorf("unknown slug: %s", slug)
	}
	f, err := os.Open(filepath.Join(s.root, slug, "db.json"))
	if err != nil {
		return fmt.Errorf("open db.json for %s: %w", slug, err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("decode db.json for %s: %w", slug, err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("decode db.json for %s: expected object, got %v", slug, tok)
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("decode db.json for %s: %w", slug, err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("decode db.json for %s: non-string key %v", slug, keyTok)
		}
		var html string
		if err := dec.Decode(&html); err != nil {
			return fmt.Errorf("decode db.json for %s/%s: %w", slug, key, err)
		}
		if err := fn(key, html); err != nil {
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
