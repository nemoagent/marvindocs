package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/piligrim/marvindocs/internal/docstore"
	"github.com/piligrim/marvindocs/internal/index"
	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

func TestBundleDocs(t *testing.T) {
	cfg := bundleConfig{
		LTS:     []string{"node~22_lts"},
		Bundles: map[string][]string{"js": {"node", "react"}},
	}
	got := bundleDocs(cfg, []string{"node", "node~22_lts", "node~20_lts", "react", "react~18", "go"})
	want := map[string][]string{"js": {"node", "node~22_lts", "react"}, "other": {"go"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBundleBuildInstall(t *testing.T) {
	ctx := context.Background()
	src := t.TempDir()
	files := map[string]string{
		"docs.json": `[{"name":"Alpha","slug":"alpha","type":"lib","version":"1","release":"r1","mtime":1},
			{"name":"Beta","slug":"beta","type":"lib","version":"1","release":"r1","mtime":1}]`,
		"alpha/index.json": `{"entries":[{"name":"Connect","path":"net/connect","type":"Function"}],"types":[]}`,
		"beta/index.json":  `{"entries":[{"name":"Other","path":"other","type":"Function"}],"types":[]}`,
		"bundles.yaml":     "bundles:\n  a: [alpha]\n",
	}
	for name, body := range files {
		p := filepath.Join(src, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w, err := devdocsfmt.NewDBWriter(filepath.Join(src, "alpha", devdocsfmt.DBName))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Add("net/connect", "<h1>Connect</h1><p>Opens a pooled connection.</p>"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := docstore.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	srcDB := filepath.Join(t.TempDir(), "src.db")
	idx, err := index.Open(srcDB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idx.Build(ctx, store); err != nil {
		t.Fatal(err)
	}
	idx.Close()

	out := t.TempDir()
	if err := runBundleBuild(filepath.Join(src, "bundles.yaml"), src, srcDB, out, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.tar.gz", "other.tar.gz"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatal(err)
		}
	}

	dst := t.TempDir()
	dstDB := filepath.Join(t.TempDir(), "dst.db")
	if err := runBundleInstall("", "", out, dst, dstDB, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	got, err := docstore.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	if docs := got.Docs(); len(docs) != 1 || docs[0].Slug != "alpha" {
		t.Fatalf("installed docs = %+v", docs)
	}
	if _, err := os.Stat(filepath.Join(dst, "alpha", devdocsfmt.DBName)); err != nil {
		t.Fatal(err)
	}
	if err := runBundleInstall("", "", out, dst, dstDB, []string{"other", "a"}); err != nil {
		t.Fatal(err)
	}
	if got, err = docstore.Open(dst); err != nil || len(got.Docs()) != 2 {
		t.Fatalf("after second install: %v, %v", got.Docs(), err)
	}
	di, err := index.Open(dstDB)
	if err != nil {
		t.Fatal(err)
	}
	defer di.Close()
	var n int
	if err := di.DB().QueryRow(`SELECT COUNT(*) FROM chunks_fts WHERE chunks_fts MATCH 'pooled'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("fts hits = %d, %v", n, err)
	}
}
