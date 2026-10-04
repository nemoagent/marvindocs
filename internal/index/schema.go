// Package index builds and queries the SQLite FTS5 index over the devdocs
// data loaded through package docstore.
//
// The index file contains these tables:
//
//	docs(slug PRIMARY KEY, name, type, version, release, mtime)
//	entries(id INTEGER PK, doc_slug, name, path, type)
//	pages(id INTEGER PK, doc_slug, path)
//	chunks(id INTEGER PK, page_id, off, len)
//	chunk_vecs(chunk_id INTEGER PK, vec BLOB)
//
// and two FTS5 virtual tables:
//
//	entries_fts(name, path, content='entries', content_rowid='id')
//	chunks_fts(crumb, body, content='', contentless_delete=1)
//
// entries_fts holds one row per symbol in index.json; triggers keep it in
// sync on insert/delete. chunks_fts indexes Markdown chunks of every page
// in db.json (see package chunk), each with a breadcrumb "doc > page >
// heading", but stores no text: its rowid equals chunks.id, and chunks
// hold only byte offsets into the page's Markdown, which is re-sliced
// from db.json at query time (see SearchPages, SearchChunks). The
// builder manages pages/chunks/chunks_fts directly.
//
// chunk_vecs optionally holds one int8-quantised embedding per chunk
// (see embed.Quantize), filled by EmbedDoc and used by SearchChunksVec.
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

CREATE TABLE IF NOT EXISTS chunks (
    id      INTEGER PRIMARY KEY,
    page_id INTEGER NOT NULL,
    off     INTEGER NOT NULL,
    len     INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_chunks_page ON chunks(page_id);

-- Contentless: crumb and body are tokenized but never stored, which keeps
-- the index small; chunks are re-sliced from the Markdown of the page at
-- query time. contentless_delete lets DELETE work by rowid (SQLite >= 3.43).
CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
    crumb,
    body,
    content='',
    contentless_delete=1,
    tokenize='unicode61 remove_diacritics 2'
);

CREATE TABLE IF NOT EXISTS chunk_vecs (
    chunk_id INTEGER PRIMARY KEY,
    vec      BLOB NOT NULL
);

CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`
