package index

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"

	"github.com/piligrim/marvindocs/internal/synonyms"
)

// Index is a handle to an open SQLite index file.
type Index struct {
	db           *sql.DB
	path         string
	catalogOnce  sync.Once
	catalogSlugs map[string]bool
	catalogNames map[string]string
	catalogErr   error

	vecMu    sync.Mutex
	vecCache map[string]*docVecs // doc slug -> vectors, loaded lazily

	syn *synonyms.Thesaurus // query expansion; nil disables it
}

// Open opens (or creates) an index file and ensures the schema exists.
// The caller is responsible for calling Close.
func Open(path string) (*Index, error) {
	// Use WAL for better concurrent reads; busy_timeout to avoid spurious
	// "database is locked" errors when the indexer and server run together.
	dsn := "file:" + path + "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=1"
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open index %s: %w", path, err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	if err := dropLegacyPagesFTS(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	if err := addContentHash(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	syn, err := synonyms.Builtin()
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Index{db: db, path: path, syn: syn}, nil
}

// SetSynonyms replaces the thesaurus used to expand search queries
// (the builtin one by default); nil disables the expansion.
func (i *Index) SetSynonyms(t *synonyms.Thesaurus) {
	i.syn = t
}

// addContentHash adds docs.content_hash to an index made by an older
// version; Sync backfills the empty values.
func addContentHash(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(docs)`)
	if err != nil {
		return err
	}
	has := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		has = has || name == "content_hash"
	}
	rows.Close()
	if err := rows.Err(); err != nil || has {
		return err
	}
	_, err = db.Exec(`ALTER TABLE docs ADD COLUMN content_hash TEXT NOT NULL DEFAULT ''`)
	return err
}

// dropLegacyPagesFTS removes the page-level pages_fts table of an older
// version. Its page rows go too, so the next Sync re-indexes every doc
// into chunks_fts.
func dropLegacyPagesFTS(db *sql.DB) error {
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE name = 'pages_fts'`).Scan(&name)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = db.Exec(`DROP TABLE pages_fts`); err != nil {
		return err
	}
	_, err = db.Exec(`DELETE FROM pages`)
	return err
}

// VectorCount returns the number of stored chunk embeddings.
func (i *Index) VectorCount(ctx context.Context) (int64, error) {
	var n int64
	err := i.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chunk_vecs`).Scan(&n)
	return n, err
}

// Close closes the underlying DB handle.
func (i *Index) Close() error {
	if i.db == nil {
		return nil
	}
	return i.db.Close()
}

// Path returns the filesystem path of the index file.
func (i *Index) Path() string { return i.path }

// DB exposes the underlying *sql.DB for advanced callers (tests, search).
func (i *Index) DB() *sql.DB { return i.db }

// Reset drops all rows from docs/entries/pages/chunks/chunk_vecs and
// rebuilds the FTS tables. Safe to call on an empty index.
func (i *Index) Reset() error {
	stmts := []string{
		`DELETE FROM entries`,
		`DELETE FROM docs`,
		`INSERT INTO entries_fts(entries_fts) VALUES('rebuild')`,
		`DELETE FROM pages`,
		`DELETE FROM chunks`,
		`DELETE FROM chunk_vecs`,
		`INSERT INTO chunks_fts(chunks_fts) VALUES('delete-all')`,
	}
	for _, s := range stmts {
		if _, err := i.db.Exec(s); err != nil {
			return fmt.Errorf("reset (%s): %w", s, err)
		}
	}
	i.vecMu.Lock()
	i.vecCache = nil
	i.vecMu.Unlock()
	return nil
}

// Remove deletes the index file from disk. Intended for tests.
func Remove(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	// WAL/SHM side files
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
	return nil
}
