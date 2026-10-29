package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/piligrim/marvindocs/internal/docstore"
	"github.com/piligrim/marvindocs/internal/index"
	"gopkg.in/yaml.v3"
)

// otherBundle holds every doc no bundle in bundles.yaml lists.
const otherBundle = "other"

// bundleConfig is bundles.yaml.
type bundleConfig struct {
	LTS     []string            `yaml:"lts"`
	Bundles map[string][]string `yaml:"bundles"`
}

func runBundle(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: marvindocs bundle (build|install) ...")
		os.Exit(2)
	}
	var err error
	switch args[0] {
	case "build":
		fs := flag.NewFlagSet("bundle build", flag.ExitOnError)
		config := fs.String("config", "bundles.yaml", "Bundle definitions")
		out := fs.String("out", "dist/bundles", "Output directory for NAME.tar.gz")
		docsPath := fs.String("docs-path", "data/public/docs", "Path to devdocs data")
		indexPath := fs.String("index-path", "marvindocs.db", "SQLite index file")
		_ = fs.Parse(args[1:])
		err = runBundleBuild(*config, *docsPath, *indexPath, *out, fs.Args())
	case "install":
		fs := flag.NewFlagSet("bundle install", flag.ExitOnError)
		repo := fs.String("repo", "nemoagent/marvindocs", "GitHub repository with the bundle release")
		tag := fs.String("tag", "bundles", "Release tag")
		from := fs.String("from", "", "Directory with NAME.tar.gz instead of downloading with gh")
		docsPath := fs.String("docs-path", "data/public/docs", "Path to devdocs data")
		indexPath := fs.String("index-path", "marvindocs.db", "SQLite index file")
		_ = fs.Parse(args[1:])
		if fs.NArg() == 0 {
			fmt.Fprintln(os.Stderr, "usage: marvindocs bundle install [flags] NAME...")
			os.Exit(2)
		}
		err = runBundleInstall(*repo, *tag, *from, *docsPath, *indexPath, fs.Args())
	default:
		fmt.Fprintf(os.Stderr, "unknown bundle command: %s\n", args[0])
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// bundleDocs maps every bundle (and otherBundle) to the slugs it ships:
// the newest version of each listed doc, the LTS releases and variants.
func bundleDocs(cfg bundleConfig, slugs []string) map[string][]string {
	owner := make(map[string]string)
	for name, docs := range cfg.Bundles {
		for _, d := range docs {
			owner[d] = name
		}
	}
	lts := make(map[string]bool, len(cfg.LTS))
	for _, s := range cfg.LTS {
		lts[s] = true
	}
	all := make(map[string][]string)
	for _, s := range slugs {
		name, ok := owner[strings.SplitN(s, "~", 2)[0]]
		if !ok {
			name = otherBundle
		}
		all[name] = append(all[name], s)
	}
	out := make(map[string][]string, len(all))
	for name, s := range all {
		out[name] = index.NewestDocs(s, lts)
	}
	return out
}

func runBundleBuild(configPath, docsPath, indexPath, outDir string, names []string) error {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var cfg bundleConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("parse %s: %w", configPath, err)
	}
	store, err := docstore.Open(docsPath)
	if err != nil {
		return err
	}
	var slugs []string
	for _, d := range store.Docs() {
		slugs = append(slugs, d.Slug)
	}
	docs := bundleDocs(cfg, slugs)
	if len(names) == 0 {
		for name := range docs {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	for _, name := range names {
		if _, ok := docs[name]; !ok {
			return fmt.Errorf("unknown bundle %q", name)
		}
	}
	manifest, err := readManifest(docsPath)
	if err != nil {
		return err
	}
	idx, err := index.Open(indexPath)
	if err != nil {
		return err
	}
	defer idx.Close()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	ctx := context.Background()
	for _, name := range names {
		fmt.Printf("bundle %s: %d docs\n", name, len(docs[name]))
		dbPath := filepath.Join(outDir, name+".db")
		if err := idx.Export(ctx, store, docs[name], dbPath); err != nil {
			return fmt.Errorf("bundle %s: %w", name, err)
		}
		archive := filepath.Join(outDir, name+".tar.gz")
		err := writeBundleArchive(archive, dbPath, docsPath, docs[name], manifest)
		_ = index.Remove(dbPath)
		if err != nil {
			return fmt.Errorf("bundle %s: %w", name, err)
		}
		if st, err := os.Stat(archive); err == nil {
			fmt.Printf("  %s %.0f MB\n", archive, float64(st.Size())/1e6)
		}
	}
	return nil
}

// manifestEntry is one docs.json entry kept verbatim, with its slug.
type manifestEntry struct {
	slug string
	raw  json.RawMessage
}

// readManifest reads docs.json; a missing file is an empty manifest.
func readManifest(docsPath string) ([]manifestEntry, error) {
	raw, err := os.ReadFile(filepath.Join(docsPath, "docs.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("decode docs.json: %w", err)
	}
	out := make([]manifestEntry, 0, len(list))
	for _, r := range list {
		var d struct {
			Slug string `json:"slug"`
		}
		if err := json.Unmarshal(r, &d); err != nil {
			return nil, fmt.Errorf("decode docs.json: %w", err)
		}
		out = append(out, manifestEntry{d.Slug, r})
	}
	return out, nil
}

func encodeManifest(m []manifestEntry) ([]byte, error) {
	list := make([]json.RawMessage, len(m))
	for i, e := range m {
		list[i] = e.raw
	}
	return json.Marshal(list)
}

// writeBundleArchive packs the bundle index as bundle.db, the docs.json
// entries of slugs as docs.json and their directories under docs/.
func writeBundleArchive(out, dbPath, docsPath string, slugs []string, manifest []manifestEntry) (err error) {
	want := make(map[string]bool, len(slugs))
	for _, s := range slugs {
		want[s] = true
	}
	var sub []manifestEntry
	for _, e := range manifest {
		if want[e.slug] {
			sub = append(sub, e)
		}
	}
	docsJSON, err := encodeManifest(sub)
	if err != nil {
		return err
	}

	f, err := os.Create(out + ".part")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(out + ".part")
		}
	}()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	addFile := func(name, path string) error {
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		st, err := src.Stat()
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: st.Size(), ModTime: st.ModTime()}); err != nil {
			return err
		}
		_, err = io.Copy(tw, src)
		return err
	}
	if err := addFile("bundle.db", dbPath); err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: "docs.json", Mode: 0o644, Size: int64(len(docsJSON))}); err != nil {
		return err
	}
	if _, err := tw.Write(docsJSON); err != nil {
		return err
	}
	for _, s := range slugs {
		dir := filepath.Join(docsPath, s)
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, err := filepath.Rel(docsPath, path)
			if err != nil {
				return err
			}
			return addFile("docs/"+filepath.ToSlash(rel), path)
		})
		if err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(out+".part", out)
}

func runBundleInstall(repo, tag, from, docsPath, indexPath string, names []string) error {
	tmp, err := os.MkdirTemp("", "marvindocs-bundle-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := os.MkdirAll(docsPath, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		return err
	}
	idx, err := index.Open(indexPath)
	if err != nil {
		return err
	}
	defer idx.Close()
	for _, name := range names {
		archive := filepath.Join(from, name+".tar.gz")
		if from == "" {
			fmt.Printf("download %s from %s@%s\n", name, repo, tag)
			cmd := exec.Command("gh", "release", "download", tag, "-R", repo,
				"-p", name+".tar.gz", "-D", tmp, "--clobber")
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("download bundle %s: %w", name, err)
			}
			archive = filepath.Join(tmp, name+".tar.gz")
		}
		n, err := installArchive(archive, tmp, docsPath, idx)
		if err != nil {
			return fmt.Errorf("install bundle %s: %w", name, err)
		}
		if from == "" {
			_ = os.Remove(archive)
		}
		fmt.Printf("bundle %s: %d docs installed\n", name, n)
	}
	fmt.Println("restart marvindocs serve to pick up the new docs")
	return nil
}

// installArchive unpacks the doc directories of a bundle into docsPath
// (replacing existing ones), merges its index into idx and adds its
// entries to docs.json. Returns the number of docs installed.
func installArchive(archive, tmp, docsPath string, idx *index.Index) (int, error) {
	f, err := os.Open(archive)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	tr := tar.NewReader(gz)
	dbPath := filepath.Join(tmp, "bundle.db")
	defer index.Remove(dbPath)
	var added []manifestEntry
	replaced := make(map[string]bool)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		switch {
		case h.Name == "bundle.db":
			if err := writeFrom(dbPath, tr); err != nil {
				return 0, err
			}
		case h.Name == "docs.json":
			raw, err := io.ReadAll(tr)
			if err != nil {
				return 0, err
			}
			if err := os.WriteFile(filepath.Join(tmp, "docs.json"), raw, 0o644); err != nil {
				return 0, err
			}
			if added, err = readManifest(tmp); err != nil {
				return 0, err
			}
		case strings.HasPrefix(h.Name, "docs/"):
			rel := filepath.FromSlash(strings.TrimPrefix(h.Name, "docs/"))
			slug, _, ok := strings.Cut(filepath.ToSlash(rel), "/")
			if !ok || !filepath.IsLocal(rel) {
				return 0, fmt.Errorf("bad path in archive: %s", h.Name)
			}
			if !replaced[slug] {
				if err := os.RemoveAll(filepath.Join(docsPath, slug)); err != nil {
					return 0, err
				}
				replaced[slug] = true
			}
			if err := writeFrom(filepath.Join(docsPath, rel), tr); err != nil {
				return 0, err
			}
		}
	}

	slugs, err := idx.Merge(context.Background(), dbPath)
	if err != nil {
		return 0, err
	}
	manifest, err := readManifest(docsPath)
	if err != nil {
		return 0, err
	}
	pos := make(map[string]int, len(manifest))
	for i, e := range manifest {
		pos[e.slug] = i
	}
	for _, e := range added {
		if i, ok := pos[e.slug]; ok {
			manifest[i] = e
		} else {
			manifest = append(manifest, e)
		}
	}
	raw, err := encodeManifest(manifest)
	if err != nil {
		return 0, err
	}
	if err := writeFrom(filepath.Join(docsPath, "docs.json"), strings.NewReader(string(raw))); err != nil {
		return 0, err
	}
	return len(slugs), nil
}

// writeFrom writes r to path through a temporary file, creating parent
// directories.
func writeFrom(path string, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path + ".part")
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(path + ".part")
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(path+".part", path)
}
