// Package main is the marvindocs CLI entrypoint.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/piligrim/marvindocs/internal/docstore"
	"github.com/piligrim/marvindocs/internal/index"
	"github.com/piligrim/marvindocs/internal/mcpserver"
	"github.com/piligrim/marvindocs/internal/web"
)

const usage = `marvindocs - MCP server and web UI for the DevDocs archive.

Usage:
  marvindocs serve [--http ADDR] [--stdio] [--docs-path DIR] [--index-path FILE]
  marvindocs index build [--fulltext] [--docs-path DIR] [--index-path FILE]
  marvindocs index stats [--index-path FILE]
  marvindocs version

Examples:
  marvindocs serve --http :3333
  marvindocs serve --stdio
  marvindocs index build --fulltext
`

// version is overridden at build time with -ldflags.
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
	case "index":
		runIndex(os.Args[2:])
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
	httpAddr := fs.String("http", "", "HTTP listen address (e.g. :3333). Empty = disabled")
	stdio := fs.Bool("stdio", false, "Run MCP server over stdio")
	docsPath := fs.String("docs-path", "data/public/docs", "Path to the devdocs data directory")
	indexPath := fs.String("index-path", "marvindocs.db", "Path to the SQLite index file")
	_ = fs.Parse(args)

	if *httpAddr == "" && !*stdio {
		fmt.Fprintln(os.Stderr, "error: specify --http ADDR and/or --stdio")
		os.Exit(2)
	}
	if err := runServeWith(*httpAddr, *stdio, *docsPath, *indexPath); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runServeWith(httpAddr string, stdio bool, docsPath, indexPath string) error {
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

	srv := mcpserver.New(mcpserver.Deps{Store: store, Index: idx}, version)

	errCh := make(chan error, 2)

	if stdio {
		go func() {
			fmt.Fprintln(os.Stderr, "marvindocs MCP stdio server: ready")
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

		httpServer = &http.Server{Addr: httpAddr, Handler: mux}

		go func() {
			fmt.Fprintf(os.Stderr, "marvindocs HTTP server: listening on %s (web at /, MCP at /mcp)\n", httpAddr)
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
		shutdownCtx, cancelShutdown := context.WithCancel(context.Background())
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
		fullText := fs.Bool("fulltext", false, "Also index page body text (adds GBs) - not implemented yet")
		docsPath := fs.String("docs-path", "data/public/docs", "Path to devdocs data")
		indexPath := fs.String("index-path", "marvindocs.db", "Output SQLite file")
		_ = fs.Parse(args[1:])
		if *fullText {
			fmt.Fprintln(os.Stderr, "warning: --fulltext is not implemented yet, ignoring")
		}
		if err := runIndexBuild(*docsPath, *indexPath); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "stats":
		fs := flag.NewFlagSet("index stats", flag.ExitOnError)
		indexPath := fs.String("index-path", "marvindocs.db", "SQLite file")
		_ = fs.Parse(args[1:])
		if err := runIndexStats(*indexPath); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: index %s\n", args[0])
		os.Exit(2)
	}
}

func runIndexBuild(docsPath, indexPath string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := docstore.Open(docsPath)
	if err != nil {
		return fmt.Errorf("open docstore: %w", err)
	}
	fmt.Fprintf(os.Stderr, "loaded docs.json: %d docs from %s\n", len(store.Docs()), docsPath)

	idx, err := index.Open(indexPath)
	if err != nil {
		return err
	}
	defer idx.Close()

	stats, err := idx.Build(ctx, store)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "indexed %d docs, %d entries in %s -> %s\n",
		stats.Docs, stats.Entries, stats.Duration.Round(1e6), indexPath)
	return nil
}

func runIndexStats(indexPath string) error {
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
	return nil
}
