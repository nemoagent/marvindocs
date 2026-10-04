package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"runtime"
	"strings"
	"sync"

	"github.com/piligrim/marvindocs/internal/chunk"
	"github.com/piligrim/marvindocs/internal/docstore"
)

// pageWorkers picks a sensible level of CPU parallelism for the
// HTML→Markdown stage of full-text indexing.
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

// indexDocPages streams every page of slug from db.json, converts them to
// Markdown and cuts them into chunks in parallel, and inserts each
// non-empty page into pages, its chunks into chunks (offsets only) and
// chunks_fts (sharing the chunk rowid). Returns the number of inserted
// pages.
//
// Errors from the docstore stream (other than a missing db.json) or SQL
// insert are surfaced through the returned error; a page that fails to
// convert is skipped. Worker goroutines exit as soon as ctx is cancelled
// or any error is observed.
func indexDocPages(ctx context.Context, s *docstore.Store, slug string, tx *sql.Tx) (int, error) {
	pageStmt, err := tx.PrepareContext(ctx, `INSERT INTO pages(doc_slug, path) VALUES (?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare pages: %w", err)
	}
	defer pageStmt.Close()
	chunkStmt, err := tx.PrepareContext(ctx, `INSERT INTO chunks(page_id, off, len) VALUES (?, ?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare chunks: %w", err)
	}
	defer chunkStmt.Close()
	ftsStmt, err := tx.PrepareContext(ctx, `INSERT INTO chunks_fts(rowid, crumb, body) VALUES (?, ?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare chunks_fts: %w", err)
	}
	defer ftsStmt.Close()
	docName := strings.SplitN(slug, "~", 2)[0]

	type job struct {
		path string
		html string
	}
	type result struct {
		path   string
		md     string
		title  string
		chunks []chunk.Chunk
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
				md, err := docstore.ToMarkdown(j.html)
				if err != nil || strings.TrimSpace(md) == "" {
					continue
				}
				r := result{path: j.path, md: md, title: chunk.Title(md), chunks: chunk.Chunks(md)}
				select {
				case results <- r:
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
		writeErr = insertPage(ctx, pageStmt, chunkStmt, ftsStmt, slug, docName, r.path, r.title, r.md, r.chunks)
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

// insertPage adds one page row, then a chunks row and a chunks_fts row
// (same rowid) per chunk. The breadcrumb is "doc > page title > heading".
func insertPage(ctx context.Context, pageStmt, chunkStmt, ftsStmt *sql.Stmt,
	slug, docName, path, title, md string, chunks []chunk.Chunk) error {
	res, err := pageStmt.ExecContext(ctx, slug, path)
	if err != nil {
		return fmt.Errorf("insert page %s/%s: %w", slug, path, err)
	}
	pageID, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("insert page %s/%s: %w", slug, path, err)
	}
	for _, c := range chunks {
		res, err := chunkStmt.ExecContext(ctx, pageID, c.Off, c.Len)
		if err != nil {
			return fmt.Errorf("insert chunk %s/%s: %w", slug, path, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("insert chunk %s/%s: %w", slug, path, err)
		}
		if _, err := ftsStmt.ExecContext(ctx, id, crumbOf(docName, title, c.Heading), md[c.Off:c.Off+c.Len]); err != nil {
			return fmt.Errorf("insert chunk body %s/%s: %w", slug, path, err)
		}
	}
	return nil
}

// crumbOf builds the breadcrumb "doc > page title > heading" of a chunk;
// empty parts and a heading equal to the title are left out.
func crumbOf(docName, title, heading string) string {
	crumb := []string{docName}
	if title != "" {
		crumb = append(crumb, title)
	}
	if heading != "" && heading != title {
		crumb = append(crumb, heading)
	}
	return strings.Join(crumb, " > ")
}
