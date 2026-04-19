// Package index builds and queries the SQLite FTS5 index over the devdocs
// data loaded through package docstore.
//
// The index file contains two tables:
//
//	docs(slug PRIMARY KEY, name, type, version, release, mtime)
//	entries(id INTEGER PK, doc_slug, name, path, type)
//
// and a contentless FTS5 virtual table:
//
//	entries_fts(name, path, content='entries', content_rowid='id')
//
// Triggers keep entries_fts in sync with entries on insert/delete. The FTS
// table is used by Search for name+path matching with BM25 ranking.
package index

// schemaSQL is executed once when a new index file is created.
const schemaSQL = `
CREATE TABLE IF NOT EXISTS docs (
    slug     TEXT PRIMARY KEY,
    name     TEXT NOT NULL,
    type     TEXT NOT NULL,
    version  TEXT NOT NULL,
    release  TEXT NOT NULL,
    mtime    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS entries (
    id       INTEGER PRIMARY KEY,
    doc_slug TEXT NOT NULL,
    name     TEXT NOT NULL,
    path     TEXT NOT NULL,
    type     TEXT NOT NULL,
    FOREIGN KEY (doc_slug) REFERENCES docs(slug)
);

CREATE INDEX IF NOT EXISTS idx_entries_doc ON entries(doc_slug);

CREATE VIRTUAL TABLE IF NOT EXISTS entries_fts USING fts5(
    name,
    path,
    content='entries',
    content_rowid='id',
    tokenize='unicode61 remove_diacritics 2'
);

CREATE TRIGGER IF NOT EXISTS entries_ai AFTER INSERT ON entries BEGIN
    INSERT INTO entries_fts(rowid, name, path) VALUES (new.id, new.name, new.path);
END;

CREATE TRIGGER IF NOT EXISTS entries_ad AFTER DELETE ON entries BEGIN
    INSERT INTO entries_fts(entries_fts, rowid, name, path) VALUES ('delete', old.id, old.name, old.path);
END;

CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`
