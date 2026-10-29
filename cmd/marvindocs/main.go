// Package main is the marvindocs CLI entrypoint.
package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/piligrim/marvindocs/internal/docstore"
	"github.com/piligrim/marvindocs/internal/embed"
	"github.com/piligrim/marvindocs/internal/index"
	"github.com/piligrim/marvindocs/internal/logging"
	"github.com/piligrim/marvindocs/internal/mcpserver"
	"github.com/piligrim/marvindocs/internal/mdcache"
	"github.com/piligrim/marvindocs/internal/synonyms"
	"github.com/piligrim/marvindocs/internal/web"
)

const usage = `marvindocs - MCP server and web UI for the DevDocs archive.

Usage:
  marvindocs serve [--http ADDR] [--stdio] [--auth-token TOKEN] [--insecure] [--docs-path DIR] [--index-path FILE]
                   [--embedder none|ollama|openai|onnx] [--ollama-url URL] [--ollama-model NAME]
                   [--openai-url URL] [--openai-model NAME] [--model-dir DIR] [--onnxruntime PATH]
  marvindocs index build [--full [--drop-vectors]] [--rehash] [--docs-path DIR] [--index-path FILE]
  marvindocs index stats [--index-path FILE] [--by-doc] [--top N]
  marvindocs index embed [--doc SLUG]... [--embedder ollama|openai|onnx] [--ollama-url URL] [--ollama-model NAME]
                         [--openai-url URL] [--openai-model NAME] [--model-dir DIR] [--onnxruntime PATH] [--batch N] [--docs-path DIR] [--index-path FILE]
                         [--all-versions]
                         (without --doc only the newest 3 versions, LTS releases and variants of a doc are embedded;
                          --all-versions embeds every version, --doc SLUG one exact version)
                         (embedder flags default to MARVINDOCS_EMBEDDER, _OLLAMA_URL, _OLLAMA_MODEL, _OPENAI_URL, _OPENAI_MODEL,
                          _MODEL_DIR, _ONNXRUNTIME;
                          index embed defaults to onnx, which needs a build with make build-onnx)
  marvindocs index prune-vectors [--dry-run] [--index-path FILE]
                         (delete vectors of the versions index embed skips by default, then VACUUM)
  marvindocs search QUERY [--doc SLUG] [--limit N] [--json]
  marvindocs get SLUG PATH [--format FORMAT]
  marvindocs list [--json]
  marvindocs pack (--all | SLUG...) [--docs-path DIR]
                  (convert devdocs db.json into db.zip; sync-docs.sh does this per download)
  marvindocs bundle build [--config FILE] [--out DIR] [--docs-path DIR] [--index-path FILE] [NAME...]
                  (pack bundles.yaml bundles as NAME.tar.gz: pages and a ready index with vectors)
  marvindocs bundle install [--repo OWNER/REPO] [--tag TAG] [--from DIR] [--docs-path DIR] [--index-path FILE] NAME...
                  (download bundles with gh from the release and merge them into the index)
  marvindocs synonyms suggest [--docs-path DIR] [--doc SLUG]... [--min-docs N] [--min-count N] [--names-only]
                              [--synonyms DIR] [--out FILE]
  marvindocs synonyms import FILE --out DIR [--force]
                  (mine abbreviation candidates from the docs into a TSV table, review it, write *.md files)
  marvindocs version

Examples:
  marvindocs serve --http :3333
  marvindocs serve --stdio
  marvindocs index build
  marvindocs search useState --doc react
  marvindocs get react reference/react/usestate
`

// version is overridden at build time with -ldflags.
var version = "dev"

func main() {
	logging.Setup()
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
	case "index":
		runIndex(os.Args[2:])
	case "search":
		runSearch(os.Args[2:])
	case "get":
		runGet(os.Args[2:])
	case "list":
		runList(os.Args[2:])
	case "pack":
		runPack(os.Args[2:])
	case "bundle":
		runBundle(os.Args[2:])
	case "synonyms":
		runSynonyms(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("marvindocs", version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	httpAddr := fs.String("http", "", "HTTP listen address. Bare ':3333' binds 127.0.0.1 only; '0.0.0.0:3333' for public. Empty = disabled")
	stdio := fs.Bool("stdio", false, "Run MCP server over stdio")
	docsPath := fs.String("docs-path", "data/public/docs", "Path to the devdocs data directory")
	indexPath := fs.String("index-path", "marvindocs.db", "Path to the SQLite index file")
	authToken := fs.String("auth-token", "", "If set, HTTP requests must carry 'Authorization: Bearer <token>'. Reads MARVINDOCS_AUTH_TOKEN if empty")
	insecure := fs.Bool("insecure", false, "Allow a non-loopback --http bind without --auth-token. Also enabled by MARVINDOCS_INSECURE=1")
	newEmbedder := embedderFlags(fs, "none")
	synDir := synonymsFlag(fs)
	_ = fs.Parse(args)

	if *httpAddr == "" && !*stdio {
		fmt.Fprintln(os.Stderr, "error: specify --http ADDR and/or --stdio")
		os.Exit(2)
	}
	token := *authToken
	if token == "" {
		token = os.Getenv("MARVINDOCS_AUTH_TOKEN")
	}
	addr := normalizeListenAddr(*httpAddr)
	if addr != "" && token == "" && !*insecure && os.Getenv("MARVINDOCS_INSECURE") != "1" && !isLoopbackAddr(addr) {
		fmt.Fprintf(os.Stderr, "error: --http %s is reachable from other hosts but no auth token is set; pass --auth-token (or MARVINDOCS_AUTH_TOKEN), or --insecure to serve without auth\n", addr)
		os.Exit(2)
	}
	e, err := newEmbedder()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := runServeWith(addr, *stdio, *docsPath, *indexPath, token, *synDir, e); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// normalizeListenAddr keeps explicit hosts ('0.0.0.0:3333',
// '192.168.1.5:3333') verbatim but rewrites the bare-port shorthand
// (':3333') to bind localhost only. This avoids accidentally exposing
// the unauthenticated server on every interface.
func normalizeListenAddr(addr string) string {
	if addr == "" {
		return ""
	}
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}

// isLoopbackAddr reports whether a listen address binds only the local
// machine: 'localhost' or a loopback IP such as 127.0.0.1 / ::1. An
// unparseable host is treated as non-loopback so the auth check errs on
// the safe side.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// bearerAuthHandler enforces 'Authorization: Bearer <token>' on every
// HTTP request except /healthz (which stays open for liveness probes).
// Matching is constant-time to avoid timing leaks of the token.
func bearerAuthHandler(token string, next http.Handler) http.Handler {
	expected := "Bearer " + token
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		got := r.Header.Get("Authorization")
		if len(got) != len(expected) || subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="marvindocs"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func runServeWith(httpAddr string, stdio bool, docsPath, indexPath, authToken, synDir string, e embed.Embedder) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e != nil {
		defer func() { _ = e.Close() }()
	}

	if _, err := os.Stat(indexPath); err != nil {
		return fmt.Errorf("index file not found: %s (run `marvindocs index build` first)", indexPath)
	}
	store, err := docstore.Open(docsPath)
	if err != nil {
		return fmt.Errorf("open docstore: %w", err)
	}
	idx, err := index.Open(indexPath)
	if err != nil {
		return err
	}
	defer idx.Close()
	if err := applySynonyms(idx, synDir); err != nil {
		return err
	}

	// Cache HTML→Markdown for hot pages. 1024 entries ~= a few MB at
	// typical doc page sizes; tuned for MCP agents that re-fetch the
	// same APIs many times in a session.
	mdc := mdcache.New(1024)

	srv := mcpserver.New(mcpserver.Deps{Store: store, Index: idx, MDCache: mdc, Embedder: e}, version)

	errCh := make(chan error, 2)

	if stdio {
		go func() {
			slog.Info("MCP stdio server ready")
			errCh <- srv.Run(ctx, &mcp.StdioTransport{})
		}()
	}

	var httpServer *http.Server
	if httpAddr != "" {
		webHandler, err := web.Handler(web.Deps{Store: store, Index: idx})
		if err != nil {
			return fmt.Errorf("init web: %w", err)
		}
		mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return srv
		}, nil)

		mux := http.NewServeMux()
		mux.Handle("/mcp", mcpHandler)
		mux.Handle("/mcp/", mcpHandler)
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
		// Web UI catches everything else (registered last so that /mcp and
		// /healthz take precedence on the ServeMux pattern tree).
		mux.Handle("/", webHandler)

		handler := web.AccessLogHandler(mux)
		if authToken != "" {
			handler = bearerAuthHandler(authToken, handler)
			slog.Info("HTTP auth enabled (Bearer)")
		}
		httpServer = &http.Server{
			Addr:    httpAddr,
			Handler: handler,
			// Bound idle and slow-header connections. No WriteTimeout:
			// /mcp keeps long-lived SSE streams open.
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		}

		go func() {
			slog.Info("HTTP server listening", "addr", httpAddr, "web", "/", "mcp", "/mcp")
			err := httpServer.ListenAndServe()
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			errCh <- err
		}()
	}

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			return err
		}
	}

	if httpServer != nil {
		// Give in-flight requests up to 10s to finish (longer than the
		// slowest realistic get_page conversion). Past that, force-close.
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelShutdown()
		_ = httpServer.Shutdown(shutdownCtx)
	}
	return nil
}

func runIndex(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: marvindocs index (build|stats|embed) ...")
		os.Exit(2)
	}
	switch args[0] {
	case "build":
		fs := flag.NewFlagSet("index build", flag.ExitOnError)
		full := fs.Bool("full", false, "Force full rebuild instead of incremental sync (default is incremental by mtime)")
		docsPath := fs.String("docs-path", "data/public/docs", "Path to devdocs data")
		dropVectors := fs.Bool("drop-vectors", false, "With --full: allow discarding all chunk embeddings (chunk_vecs). Without it --full refuses when vectors exist")
		rehash := fs.Bool("rehash", false, "Recompute content hashes of unchanged docs without re-indexing them (after `pack --all`)")
		indexPath := fs.String("index-path", "marvindocs.db", "Output SQLite file")
		_ = fs.Parse(args[1:])
		if err := runIndexBuild(*docsPath, *indexPath, *full, *dropVectors, *rehash); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "stats":
		fs := flag.NewFlagSet("index stats", flag.ExitOnError)
		indexPath := fs.String("index-path", "marvindocs.db", "SQLite file")
		byDoc := fs.Bool("by-doc", false, "Print per-doc breakdown sorted by entry count")
		topN := fs.Int("top", 0, "Limit per-doc output to top N rows (0 = all)")
		_ = fs.Parse(args[1:])
		if err := runIndexStats(*indexPath, *byDoc, *topN); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "embed":
		fs := flag.NewFlagSet("index embed", flag.ExitOnError)
		var docs []string
		fs.Func("doc", "Doc slug to embed; may be repeated (default: the newest 3 versions and LTS of every doc)", func(v string) error {
			docs = append(docs, v)
			return nil
		})
		newEmbedder := embedderFlags(fs, "onnx")
		batch := fs.Int("batch", 64, "Chunks per embedder call and per transaction")
		allVersions := fs.Bool("all-versions", false, "Without --doc embed every version, not only the newest 3 and LTS")
		docsPath := fs.String("docs-path", "data/public/docs", "Path to devdocs data")
		indexPath := fs.String("index-path", "marvindocs.db", "SQLite index file")
		_ = fs.Parse(args[1:])
		e, err := newEmbedder()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if e == nil {
			fmt.Fprintln(os.Stderr, "error: index embed needs --embedder ollama, openai or onnx")
			os.Exit(2)
		}
		if err := runIndexEmbed(*docsPath, *indexPath, docs, *allVersions, e, *batch); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "prune-vectors":
		fs := flag.NewFlagSet("index prune-vectors", flag.ExitOnError)
		dryRun := fs.Bool("dry-run", false, "Only list the docs whose vectors would be deleted")
		indexPath := fs.String("index-path", "marvindocs.db", "SQLite index file")
		_ = fs.Parse(args[1:])
		if err := runIndexPruneVectors(*indexPath, *dryRun); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: index %s\n", args[0])
		os.Exit(2)
	}
}

func runIndexBuild(docsPath, indexPath string, full, dropVectors, rehash bool) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := docstore.Open(docsPath)
	if err != nil {
		return fmt.Errorf("open docstore: %w", err)
	}
	slog.Info("loaded docstore", "docs", len(store.Docs()), "path", docsPath)

	idx, err := index.Open(indexPath)
	if err != nil {
		return err
	}
	defer idx.Close()

	if full && !dropVectors {
		n, err := idx.VectorCount(ctx)
		if err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("--full would discard %d chunk embeddings in %s; re-embedding takes hours. "+
				"Use `index build` (incremental, keeps vectors) or add --drop-vectors to confirm", n, indexPath)
		}
	}

	var stats index.BuildStats
	if full {
		stats, err = idx.Build(ctx, store)
	} else {
		stats, err = idx.Sync(ctx, store, rehash)
	}
	if err != nil {
		return err
	}
	// Deletes (sync, --full) only move pages to the freelist; without
	// VACUUM the file never shrinks.
	if reclaimed, err := idx.Compact(ctx); err != nil {
		slog.Warn("index compact failed", "err", err)
	} else if reclaimed > 0 {
		slog.Info("index compacted", "reclaimed_mb", reclaimed>>20)
	}
	mode := "synced"
	if full {
		mode = "indexed"
	}
	slog.Info("index "+mode,
		"docs", stats.Docs,
		"entries", stats.Entries,
		"pages", stats.Pages,
		"touched", stats.Touched,
		"hash_backfilled", stats.HashBackfilled,
		"duration", stats.Duration.Round(1e6).String(),
		"path", indexPath)
	return nil
}

// envOr returns the environment variable key, or def when it is unset or empty.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// synonymsFlag registers --synonyms on fs; see applySynonyms.
func synonymsFlag(fs *flag.FlagSet) *string {
	return fs.String("synonyms", envOr("MARVINDOCS_SYNONYMS", ""), "Directory of extra synonym files (*.md with 'synonyms::' lines) added to the builtin ones; 'off' disables query expansion; env MARVINDOCS_SYNONYMS")
}

// applySynonyms sets the query thesaurus of idx from a --synonyms value:
// empty keeps the builtin one, "off" disables it, a directory adds its
// files to the builtin ones.
func applySynonyms(idx *index.Index, val string) error {
	switch val {
	case "":
		return nil
	case "off":
		idx.SetSynonyms(nil)
		return nil
	}
	th, err := synonyms.Builtin()
	if err != nil {
		return err
	}
	if err := th.LoadDir(val); err != nil {
		return err
	}
	idx.SetSynonyms(th)
	return nil
}

// embedderFlags registers the embedder flags on fs with def as the
// default kind and returns a function that builds the chosen embedder;
// it returns nil for kind "none".
func embedderFlags(fs *flag.FlagSet, def string) func() (embed.Embedder, error) {
	kind := fs.String("embedder", envOr("MARVINDOCS_EMBEDDER", def), "Embedder: none, ollama, openai or onnx (onnx needs `make build-onnx`); env MARVINDOCS_EMBEDDER")
	ollamaURL := fs.String("ollama-url", envOr("MARVINDOCS_OLLAMA_URL", "http://localhost:11434"), "Ollama base URL; env MARVINDOCS_OLLAMA_URL")
	ollamaModel := fs.String("ollama-model", envOr("MARVINDOCS_OLLAMA_MODEL", "bge-m3"), "Ollama model name; env MARVINDOCS_OLLAMA_MODEL")
	openaiURL := fs.String("openai-url", envOr("MARVINDOCS_OPENAI_URL", "http://localhost:8000/v1"), "OpenAI-compatible API base URL with /v1 (vLLM); env MARVINDOCS_OPENAI_URL")
	openaiModel := fs.String("openai-model", envOr("MARVINDOCS_OPENAI_MODEL", "bge-m3"), "OpenAI-compatible embedding model name; env MARVINDOCS_OPENAI_MODEL")
	modelDir := fs.String("model-dir", envOr("MARVINDOCS_MODEL_DIR", "data/onnx/model"), "onnx: directory with model_int8.onnx and tokenizer.json; env MARVINDOCS_MODEL_DIR")
	ortLib := fs.String("onnxruntime", envOr("MARVINDOCS_ONNXRUNTIME", "data/onnx/lib/libonnxruntime.so"), "onnx: path to the ONNX Runtime shared library; env MARVINDOCS_ONNXRUNTIME")
	return func() (embed.Embedder, error) {
		switch *kind {
		case "none":
			return nil, nil
		case "ollama":
			return embed.NewOllama(*ollamaURL, *ollamaModel), nil
		case "openai":
			return embed.NewOpenAI(*openaiURL, *openaiModel), nil
		case "onnx":
			return embed.NewONNX(*modelDir, *ortLib)
		}
		return nil, fmt.Errorf("unknown embedder %q (want none, ollama, openai or onnx)", *kind)
	}
}

func runIndexEmbed(docsPath, indexPath string, docs []string, allVersions bool, e embed.Embedder, batch int) error {
	defer func() { _ = e.Close() }()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if _, err := os.Stat(indexPath); err != nil {
		return fmt.Errorf("index file not found: %s (run `marvindocs index build` first)", indexPath)
	}
	store, err := docstore.Open(docsPath)
	if err != nil {
		return fmt.Errorf("open docstore: %w", err)
	}
	idx, err := index.Open(indexPath)
	if err != nil {
		return err
	}
	defer idx.Close()

	if len(docs) == 0 && !allVersions {
		if docs, err = idx.VectorDocs(ctx); err != nil {
			return err
		}
	}

	start := time.Now()
	last := start
	progress := func(done, total int) {
		if time.Since(last) < 2*time.Second && done < total {
			return
		}
		last = time.Now()
		fmt.Fprintf(os.Stderr, "embedded %d/%d chunks (%.1f chunks/s)\n",
			done, total, float64(done)/time.Since(start).Seconds())
	}
	stats, err := idx.EmbedAll(ctx, store, docs, e, batch, progress)
	if errors.Is(err, context.Canceled) {
		slog.Warn("interrupted; embedded chunks are kept")
		err = nil
	}
	slog.Info("index embedded",
		"chunks", stats.Chunks,
		"pages", stats.Pages,
		"duration", time.Since(start).Round(time.Millisecond).String(),
		"path", indexPath)
	return err
}

func runIndexPruneVectors(indexPath string, dryRun bool) error {
	if _, err := os.Stat(indexPath); err != nil {
		return fmt.Errorf("index file not found: %s", indexPath)
	}
	idx, err := index.Open(indexPath)
	if err != nil {
		return err
	}
	defer idx.Close()
	ctx := context.Background()
	pruned, err := idx.PruneVectors(ctx, dryRun)
	if err != nil {
		return err
	}
	slugs := make([]string, 0, len(pruned))
	total := 0
	for s, n := range pruned {
		slugs = append(slugs, s)
		total += n
	}
	sort.Strings(slugs)
	for _, s := range slugs {
		fmt.Printf("%s\t%d\n", s, pruned[s])
	}
	verb := "deleted"
	if dryRun {
		verb = "would delete"
	}
	fmt.Printf("%s %d vectors of %d docs\n", verb, total, len(slugs))
	if dryRun {
		return nil
	}
	// Vacuum even when nothing was deleted now: a previous run may have been
	// interrupted after the deletes.
	fmt.Println("vacuum...")
	return idx.Vacuum(ctx)
}

func runIndexStats(indexPath string, byDoc bool, topN int) error {
	if _, err := os.Stat(indexPath); err != nil {
		return fmt.Errorf("index file not found: %s (run `marvindocs index build` first)", indexPath)
	}
	idx, err := index.Open(indexPath)
	if err != nil {
		return err
	}
	defer idx.Close()

	st, err := idx.Stats(context.Background())
	if err != nil {
		return err
	}
	fi, _ := os.Stat(indexPath)
	fmt.Printf("index:   %s\n", indexPath)
	if fi != nil {
		fmt.Printf("size:    %d bytes\n", fi.Size())
	}
	fmt.Printf("docs:    %d\n", st.Docs)
	fmt.Printf("entries: %d\n", st.Entries)
	fmt.Printf("pages:   %d\n", st.Pages)

	if byDoc {
		rows, err := idx.PerDocStats(context.Background())
		if err != nil {
			return err
		}
		if topN > 0 && topN < len(rows) {
			rows = rows[:topN]
		}
		fmt.Printf("\n%-30s  %10s  %10s\n", "SLUG", "ENTRIES", "PAGES")
		for _, r := range rows {
			fmt.Printf("%-30s  %10d  %10d\n", r.Slug, r.Entries, r.Pages)
		}
	}
	return nil
}
