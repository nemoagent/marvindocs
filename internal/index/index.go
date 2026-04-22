package index

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// Index is a handle to an open SQLite index file.
type Index struct {
	db   *sql.DB
	path string
}

// Open opens (or creates) an index file and ensures the schema exists.
// The caller is responsible for calling Close.
func Open(path string) (*Index, error) {
	// Use WAL for better concurrent reads; busy_timeout to avoid spurious
	// "database is locked" errors when the indexer and server run together.
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open index %s: %w", path, err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &Index{db: db, path: path}, nil
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

// Reset drops all rows from docs/entries/pages_fts and rebuilds the FTS
// tables. Safe to call on an empty index.
func (i *Index) Reset() error {
	stmts := []string{
		`DELETE FROM entries`,
		`DELETE FROM docs`,
		`INSERT INTO entries_fts(entries_fts) VALUES('rebuild')`,
		`DELETE FROM pages_fts`,
	}
	for _, s := range stmts {
		if _, err := i.db.Exec(s); err != nil {
			return fmt.Errorf("reset (%s): %w", s, err)
		}
	}
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
