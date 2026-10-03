// Package index builds and queries the SQLite FTS5 index over the devdocs
// data loaded through package docstore.
//
// The index file contains these tables:
//
//	docs(slug PRIMARY KEY, name, type, version, release, mtime)
//	entries(id INTEGER PK, doc_slug, name, path, type)
//	pages(id INTEGER PK, doc_slug, path)
//
// and two FTS5 virtual tables:
//
//	entries_fts(name, path, content='entries', content_rowid='id')
//	pages_fts(body, content='', contentless_delete=1)
//
// entries_fts holds one row per symbol in index.json; triggers keep it in
// sync on insert/delete. pages_fts indexes the plain-text body of every
// page in db.json but stores no text: its rowid equals pages.id, and
// snippets are rebuilt from db.json at query time (see SearchPages).
// The builder manages pages/pages_fts directly.
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
-- NOCASE so that the default (case-insensitive) LIKE 'prefix%' can use
-- it; see searchSubstring.
CREATE INDEX IF NOT EXISTS idx_entries_name ON entries(name COLLATE NOCASE);

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

CREATE TABLE IF NOT EXISTS pages (
    id       INTEGER PRIMARY KEY,
    doc_slug TEXT NOT NULL,
    path     TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_pages_doc ON pages(doc_slug);

-- Contentless: the body is tokenized but never stored, which keeps the
-- index at roughly a third of the plain-text size. contentless_delete
-- lets DELETE work by rowid (SQLite >= 3.43).
CREATE VIRTUAL TABLE IF NOT EXISTS pages_fts USING fts5(
    body,
    content='',
    contentless_delete=1,
    tokenize='unicode61 remove_diacritics 2'
);

CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`
