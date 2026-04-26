package index

import "database/sql"

// applyBulkPragmas tunes the database for fast bulk inserts. It returns
// a function that restores the rebuilt-time defaults (WAL + normal
// synchronous). Errors are intentionally ignored: PRAGMAs are advisory
// and should never block a build.
//
// Risk: with synchronous=OFF + journal_mode=MEMORY a hard crash mid-build
// can corrupt the index file. That's acceptable here because the index
// is fully rebuildable from data/public/docs.
func applyBulkPragmas(db *sql.DB) func() {
	pragmas := []string{
		`PRAGMA synchronous = OFF`,
		`PRAGMA journal_mode = MEMORY`,
		`PRAGMA temp_store = MEMORY`,
		`PRAGMA cache_size = -262144`, // ~256 MiB page cache
		`PRAGMA mmap_size = 268435456`,
	}
	for _, p := range pragmas {
		_, _ = db.Exec(p)
	}
	return func() {
		_, _ = db.Exec(`PRAGMA journal_mode = WAL`)
		_, _ = db.Exec(`PRAGMA synchronous = NORMAL`)
	}
}
