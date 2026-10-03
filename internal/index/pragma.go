package index

import (
	"database/sql"

	sqlite3 "github.com/mattn/go-sqlite3"
)

// driverName is a go-sqlite3 driver registered with a ConnectHook so the
// read-path PRAGMAs below apply to every pooled connection. cache_size,
// mmap_size and temp_store are per-connection settings, so a plain
// db.Exec would only tune whichever connection happened to run it.
const driverName = "sqlite3_marvindocs"

func init() {
	sql.Register(driverName, &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			for _, p := range readPragmas {
				if _, err := conn.Exec(p, nil); err != nil {
					return err
				}
			}
			return nil
		},
	})
}

// readPragmas speed up the serve/search path: a 64 MiB page cache and
// a 256 MiB mmap window keep hot FTS segments and the entries index in
// memory across queries; temp_store=MEMORY keeps ORDER BY sorts off disk.
var readPragmas = []string{
	`PRAGMA cache_size = -65536`,
	`PRAGMA mmap_size = 268435456`,
	`PRAGMA temp_store = MEMORY`,
}

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
