package index

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
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
	return &Index{db: db, path: path}, nil
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
