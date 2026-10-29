package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

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
	dstDB := filepath.Join(t.TempDir(), "index", "dst.db")
	if err := runBundleInstall("", out, dst, dstDB, []string{"a"}); err != nil {
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
	// The second install downloads over HTTP and checks SHA256SUMS.
	var sums string
	for _, name := range []string{"a.tar.gz", "other.tar.gz"} {
		raw, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		sums += fmt.Sprintf("%x  %s\n", sha256.Sum256(raw), name)
	}
	if err := os.WriteFile(filepath.Join(out, "SHA256SUMS"), []byte(sums), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(out)))
	defer srv.Close()
	if err := runBundleInstall(srv.URL, "", dst, dstDB, []string{"missing"}); err == nil {
		t.Fatal("install of an unknown bundle succeeded")
	}
	if err := runBundleInstall(srv.URL, "", dst, dstDB, []string{"other", "a"}); err != nil {
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

// TestDownloadResume cuts the first response in the middle: download
// retries and asks only for the missing tail with a Range header.
func TestDownloadResume(t *testing.T) {
	downloadBackoff = time.Millisecond
	data := bytes.Repeat([]byte("marvindocs bundle "), 10000)
	sum := sha256.Sum256(data)
	var calls int
	var ranges []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		ranges = append(ranges, r.Header.Get("Range"))
		if calls == 1 {
			// Promise the whole file, send half and drop the connection.
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			_, _ = w.Write(data[:len(data)/2])
			if hj, ok := w.(http.Hijacker); ok {
				if c, _, err := hj.Hijack(); err == nil {
					c.Close()
				}
			}
			return
		}
		http.ServeContent(w, r, "b.tar.gz", time.Time{}, bytes.NewReader(data))
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "b.tar.gz")
	if err := download(srv.URL, path, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("downloaded %d bytes, err %v; want %d bytes", len(got), err, len(data))
	}
	if calls != 2 || ranges[1] != fmt.Sprintf("bytes=%d-", len(data)/2) {
		t.Errorf("calls %d, ranges %q; want a resume from the middle", calls, ranges)
	}
}

func TestDownloadChecksumMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("bad"))
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "b.tar.gz")
	if err := download(srv.URL, path, strings.Repeat("0", 64)); err == nil {
		t.Fatal("want checksum error")
	}
	if _, err := os.Stat(path + ".part"); !os.IsNotExist(err) {
		t.Errorf("part file left behind: %v", err)
	}
}
