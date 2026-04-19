// Package main is the marvindocs CLI entrypoint.
package main

import (
	"flag"
	"fmt"
	"os"
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
	fmt.Fprintf(os.Stderr, "marvindocs serve http=%q stdio=%v docs=%q index=%q (not implemented yet)\n",
		*httpAddr, *stdio, *docsPath, *indexPath)
}

func runIndex(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: marvindocs index (build|stats) ...")
		os.Exit(2)
	}
	switch args[0] {
	case "build":
		fs := flag.NewFlagSet("index build", flag.ExitOnError)
		fullText := fs.Bool("fulltext", false, "Also index page body text (adds GBs)")
		docsPath := fs.String("docs-path", "data/public/docs", "Path to devdocs data")
		indexPath := fs.String("index-path", "marvindocs.db", "Output SQLite file")
		_ = fs.Parse(args[1:])
		fmt.Fprintf(os.Stderr, "index build fulltext=%v docs=%q index=%q (not implemented yet)\n",
			*fullText, *docsPath, *indexPath)
	case "stats":
		fs := flag.NewFlagSet("index stats", flag.ExitOnError)
		indexPath := fs.String("index-path", "marvindocs.db", "SQLite file")
		_ = fs.Parse(args[1:])
		fmt.Fprintf(os.Stderr, "index stats %q (not implemented yet)\n", *indexPath)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: index %s\n", args[0])
		os.Exit(2)
	}
}
