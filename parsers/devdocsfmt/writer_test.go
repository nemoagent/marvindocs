package devdocsfmt

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWriterFlush(t *testing.T) {
	dir := t.TempDir()

	w := NewWriter(Doc{
		Name:    "Demo",
		Slug:    "demo~1",
		Type:    "demo",
		Version: "1",
		Release: "1.0",
	})
	w.AddPage("index", "<h1>Demo</h1><p>hi</p>")
	w.AddPage("guide", "<h2>Guide</h2>")
	w.AddEntry("Demo", "index", "Overview")
	w.AddEntry("Guide", "guide", "Guide")
	w.AddEntry("Intro", "index#intro", "Overview")

	if err := w.Flush(filepath.Join(dir, "demo~1"), false); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "demo~1")

	for _, name := range []string{"meta.json", "index.json", DBName, ".last-modified"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}

	var meta Doc
	mustReadJSON(t, filepath.Join(out, "meta.json"), &meta)
	if meta.Slug != "demo~1" || meta.DBSize == 0 {
		t.Fatalf("bad meta: %+v", meta)
	}

	db := mustReadDB(t, filepath.Join(out, DBName))
	if db["index"] != "<h1>Demo</h1><p>hi</p>" || db["guide"] != "<h2>Guide</h2>" || len(db) != 2 {
		t.Fatalf("db.zip has wrong pages: %v", db)
	}

	var idx struct {
		Entries []Entry
		Types   []DocType
	}
	mustReadJSON(t, filepath.Join(out, "index.json"), &idx)
	if len(idx.Entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(idx.Entries))
	}
	if len(idx.Types) != 2 {
		t.Fatalf("want 2 types, got %d (%v)", len(idx.Types), idx.Types)
	}
}

func TestWriterRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(Doc{Name: "x", Slug: "x", Type: "x", Version: "1"})
	w.AddPage("p", "<p/>")
	if err := w.Flush(dir, false); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(dir, false); err == nil {
		t.Fatal("expected error on second flush without --force")
	}
	if err := w.Flush(dir, true); err != nil {
		t.Fatalf("force flush should succeed: %v", err)
	}
}

func TestWriterRequiresPagesAndMeta(t *testing.T) {
	w := NewWriter(Doc{Slug: "x"})
	if err := w.Flush(t.TempDir(), false); err == nil {
		t.Fatal("expected error: no pages")
	}
	w2 := NewWriter(Doc{})
	w2.AddPage("p", "<p/>")
	if err := w2.Flush(t.TempDir(), false); err == nil {
		t.Fatal("expected error: missing Doc.Name/Slug")
	}
}

func mustReadDB(t *testing.T, path string) map[string]string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	pages := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		pages[f.Name] = string(body)
	}
	return pages
}

func mustReadJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}
