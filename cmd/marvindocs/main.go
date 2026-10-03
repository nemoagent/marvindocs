// Package main is the marvindocs CLI entrypoint.
package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/piligrim/marvindocs/internal/docstore"
	"github.com/piligrim/marvindocs/internal/index"
	"github.com/piligrim/marvindocs/internal/logging"
	"github.com/piligrim/marvindocs/internal/mcpserver"
	"github.com/piligrim/marvindocs/internal/mdcache"
	"github.com/piligrim/marvindocs/internal/web"
)

const usage = `marvindocs - MCP server and web UI for the DevDocs archive.

Usage:
  marvindocs serve [--http ADDR] [--stdio] [--docs-path DIR] [--index-path FILE]
  marvindocs index build [--fulltext] [--full] [--docs-path DIR] [--index-path FILE]
  marvindocs index stats [--index-path FILE] [--by-doc] [--top N]
  marvindocs search QUERY [--doc SLUG] [--limit N] [--json]
  marvindocs get SLUG PATH [--format FORMAT]
  marvindocs list [--json]
  marvindocs version

Examples:
  marvindocs serve --http :3333
  marvindocs serve --stdio
  marvindocs index build --fulltext
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
	_ = fs.Parse(args)

	if *httpAddr == "" && !*stdio {
		fmt.Fprintln(os.Stderr, "error: specify --http ADDR and/or --stdio")
		os.Exit(2)
	}
	token := *authToken
	if token == "" {
		token = os.Getenv("MARVINDOCS_AUTH_TOKEN")
	}
	if err := runServeWith(normalizeListenAddr(*httpAddr), *stdio, *docsPath, *indexPath, token); err != nil {
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

func runServeWith(httpAddr string, stdio bool, docsPath, indexPath, authToken string) error {
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

	// Cache HTML→Markdown for hot pages. 1024 entries ~= a few MB at
	// typical doc page sizes; tuned for MCP agents that re-fetch the
	// same APIs many times in a session.
	mdc := mdcache.New(1024)

	srv := mcpserver.New(mcpserver.Deps{Store: store, Index: idx, MDCache: mdc}, version)

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

		var handler http.Handler = web.AccessLogHandler(mux)
		if authToken != "" {
			handler = bearerAuthHandler(authToken, handler)
			slog.Info("HTTP auth enabled (Bearer)")
		}
		httpServer = &http.Server{Addr: httpAddr, Handler: handler}

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
		fmt.Fprintln(os.Stderr, "usage: marvindocs index (build|stats) ...")
		os.Exit(2)
	}
	switch args[0] {
	case "build":
		fs := flag.NewFlagSet("index build", flag.ExitOnError)
		fullText := fs.Bool("fulltext", false, "Also index page body text into pages_fts (adds GBs, much slower)")
		full := fs.Bool("full", false, "Force full rebuild instead of incremental sync (default is incremental by mtime)")
		docsPath := fs.String("docs-path", "data/public/docs", "Path to devdocs data")
		indexPath := fs.String("index-path", "marvindocs.db", "Output SQLite file")
		_ = fs.Parse(args[1:])
		if err := runIndexBuild(*docsPath, *indexPath, *fullText, *full); err != nil {
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
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: index %s\n", args[0])
		os.Exit(2)
	}
}

func runIndexBuild(docsPath, indexPath string, fullText, full bool) error {
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

	if fullText {
		slog.Info("full-text body indexing enabled; build will be slow and index large")
	}

	opts := index.BuildOptions{Fulltext: fullText}
	var stats index.BuildStats
	if full {
		stats, err = idx.BuildWithOptions(ctx, store, opts)
	} else {
		stats, err = idx.Sync(ctx, store, opts)
	}
	if err != nil {
		return err
	}
	// Deletes (sync, --full, dropping --fulltext bodies) only move pages to
	// the freelist; without VACUUM the file never shrinks.
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
		"duration", stats.Duration.Round(1e6).String(),
		"path", indexPath)
	return nil
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
