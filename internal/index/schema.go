// Package index builds and queries the SQLite FTS5 index over the devdocs
// data loaded through package docstore.
//
// The index file contains these tables:
//
//	docs(slug PRIMARY KEY, name, type, version, release, mtime)
//	entries(id INTEGER PK, doc_slug, name, path, type)
//
// and two FTS5 virtual tables:
//
//	entries_fts(name, path, content='entries', content_rowid='id')
//	pages_fts(doc_slug UNINDEXED, path UNINDEXED, body)
//
// entries_fts is always populated (one row per symbol in index.json).
// pages_fts is populated only when Build is called with Fulltext=true;
// otherwise it stays empty. Triggers keep entries_fts in sync on
// insert/delete; pages_fts is a standalone contentless table, managed
// directly by the builder.
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

CREATE VIRTUAL TABLE IF NOT EXISTS pages_fts USING fts5(
    doc_slug UNINDEXED,
    path     UNINDEXED,
    body,
    tokenize='unicode61 remove_diacritics 2'
);

CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`
