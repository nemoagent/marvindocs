package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"runtime"
	"sync"

	"github.com/piligrim/marvindocs/internal/docstore"
)

// pageWorkers picks a sensible level of CPU parallelism for the
// HTML→plain-text stage of full-text indexing.
func pageWorkers() int {
	n := runtime.NumCPU()
	if n < 2 {
		return 1
	}
	if n > 8 {
		return 8 // diminishing returns; SQLite writer is the next bottleneck
	}
	return n
}

// indexDocPages streams every page of slug from db.json, plain-textifies
// them in parallel, and inserts each non-empty result into pages and
// pages_fts (sharing the rowid). Returns the number of inserted pages.
//
// Errors from the docstore stream (other than a missing db.json),
// plain-text conversion, or SQL insert are surfaced through the
// returned error. Worker goroutines exit as
// soon as ctx is cancelled or any error is observed.
func indexDocPages(ctx context.Context, s *docstore.Store, slug string, tx *sql.Tx) (int, error) {
	pageStmt, err := tx.PrepareContext(ctx, `INSERT INTO pages(doc_slug, path) VALUES (?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare pages: %w", err)
	}
	defer pageStmt.Close()
	bodyStmt, err := tx.PrepareContext(ctx, `INSERT INTO pages_fts(rowid, body) VALUES (?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare pages_fts: %w", err)
	}
	defer bodyStmt.Close()

	type job struct {
		path string
		html string
	}
	type result struct {
		path string
		body string
	}

	workers := pageWorkers()
	jobs := make(chan job, workers*2)
	results := make(chan result, workers*2)

	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for j := range jobs {
				if jobCtx.Err() != nil {
					return
				}
				body := docstore.ToPlainText(j.html)
				if body == "" {
					continue
				}
				select {
				case results <- result{j.path, body}:
				case <-jobCtx.Done():
					return
				}
			}
		}()
	}

	// Producer: stream db.json into jobs.
	streamErrCh := make(chan error, 1)
	go func() {
		err := s.StreamPages(slug, func(path, html string) error {
			if err := jobCtx.Err(); err != nil {
				return err
			}
			select {
			case jobs <- job{path, html}:
				return nil
			case <-jobCtx.Done():
				return jobCtx.Err()
			}
		})
		close(jobs)
		streamErrCh <- err
	}()

	// Closer: once all workers are done, close results so the writer
	// loop below exits.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Writer (this goroutine): own the SQLite stmt.
	var inserted int
	var writeErr error
	for r := range results {
		writeErr = insertPage(ctx, pageStmt, bodyStmt, slug, r.path, r.body)
		if writeErr != nil {
			cancel()
			// Drain to allow workers and producer to exit cleanly.
			for range results {
			}
			break
		}
		inserted++
	}

	streamErr := <-streamErrCh
	if writeErr != nil {
		return inserted, writeErr
	}
	// A doc without db.json ships only index.json: nothing to index.
	if streamErr != nil && !errors.Is(streamErr, fs.ErrNotExist) {
		return inserted, streamErr
	}
	return inserted, nil
}

// insertPage adds one page row and its FTS body under the same rowid.
func insertPage(ctx context.Context, pageStmt, bodyStmt *sql.Stmt, slug, path, body string) error {
	res, err := pageStmt.ExecContext(ctx, slug, path)
	if err != nil {
		return fmt.Errorf("insert page %s/%s: %w", slug, path, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("insert page %s/%s: %w", slug, path, err)
	}
	if _, err := bodyStmt.ExecContext(ctx, id, body); err != nil {
		return fmt.Errorf("insert page body %s/%s: %w", slug, path, err)
	}
	return nil
}
