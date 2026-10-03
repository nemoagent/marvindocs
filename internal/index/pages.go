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
// them in parallel, and inserts each non-empty result via pageStmt.
// Returns the number of inserted rows.
//
// Errors from the docstore stream (other than a missing db.json),
// plain-text conversion, or SQL insert are surfaced through the
// returned error. Worker goroutines exit as
// soon as ctx is cancelled or any error is observed.
func indexDocPages(ctx context.Context, s *docstore.Store, slug string, pageStmt *sql.Stmt) (int, error) {
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
		if _, err := pageStmt.ExecContext(ctx, slug, r.path, r.body); err != nil {
			writeErr = fmt.Errorf("insert page %s/%s: %w", slug, r.path, err)
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
