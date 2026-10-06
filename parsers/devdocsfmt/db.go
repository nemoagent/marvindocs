package devdocsfmt

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"time"
)

// DBName is the per-doc page archive: one deflated entry per page, the
// entry name is the page path and the body is the raw HTML fragment.
// It replaces devdocs' db.json ({path: html}) so a dump takes 4-6x less
// disk and a single page can be read without scanning the whole file.
const DBName = "db.zip"

// DBWriter streams pages into a db.zip. Pages are written in call order;
// Close finalises the archive and moves it into place atomically.
type DBWriter struct {
	path string
	f    *os.File
	zw   *zip.Writer
	size int64 // uncompressed bytes written so far
}

// NewDBWriter starts writing path (normally <doc dir>/db.zip) through a
// .tmp sibling, so a crash never leaves a truncated archive behind.
func NewDBWriter(path string) (*DBWriter, error) {
	f, err := os.Create(path + ".tmp")
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", path, err)
	}
	return &DBWriter{path: path, f: f, zw: zip.NewWriter(f)}, nil
}

// Add appends one page. Modified is fixed so the archive bytes depend
// only on the content, not on when it was packed.
func (w *DBWriter) Add(path, html string) error {
	e, err := w.zw.CreateHeader(&zip.FileHeader{
		Name:     path,
		Method:   zip.Deflate,
		Modified: time.Unix(0, 0).UTC(),
	})
	if err != nil {
		return fmt.Errorf("add page %s: %w", path, err)
	}
	if _, err := io.WriteString(e, html); err != nil {
		return fmt.Errorf("write page %s: %w", path, err)
	}
	w.size += int64(len(html))
	return nil
}

// Size returns the uncompressed byte count written so far (the value
// devdocs reports as db_size in meta.json).
func (w *DBWriter) Size() int64 { return w.size }

// Close finalises and renames the archive into place.
func (w *DBWriter) Close() error {
	tmp := w.f.Name()
	if err := w.zw.Close(); err != nil {
		w.f.Close()
		os.Remove(tmp)
		return fmt.Errorf("finish %s: %w", w.path, err)
	}
	if err := w.f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close %s: %w", w.path, err)
	}
	if err := os.Rename(tmp, w.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", w.path, err)
	}
	return nil
}
