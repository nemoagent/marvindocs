// Command go-dev-parser collects Go documentation from go.dev and
// emits a DevDocs-shaped tree.
//
// Two modes:
//
//	--mode overview  (default)
//	  Gathers the version-agnostic top-level pages from go.dev/doc:
//	  install, effective_go, faq, modules/*, database/*, tutorial/*,
//	  plus the language spec at /ref/spec. Output slug: 'go-dev'.
//
//	--mode release --version 1.22
//	  Gathers the release notes for a specific Go release from
//	  /doc/go1.22 (and similar). Output slug: 'go-dev~1.22'.
//
// Usage:
//
//	go-dev-parser --out data/public/docs/go-dev
//	go-dev-parser --mode release --version 1.22 \
//	              --out data/public/docs/go-dev~1.22
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

func main() {
	fs := flag.NewFlagSet("go-dev-parser", flag.ExitOnError)
	flags := devdocsfmt.Flags{}
	flags.RegisterFlags(fs)
	mode := fs.String("mode", "overview", "Mode: 'overview' or 'release'")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if flags.Source == "" {
		flags.Source = "https://go.dev"
	}
	if flags.Out == "" {
		devdocsfmt.Fatal("--out is required")
	}
	switch *mode {
	case "overview":
		if err := runOverview(flags); err != nil {
			devdocsfmt.Fatal("%s", err)
		}
	case "release":
		if flags.Version == "" {
			devdocsfmt.Fatal("--version (e.g. 1.22) is required for release mode")
		}
		if flags.Release == "" {
			flags.Release = flags.Version
		}
		if err := runRelease(flags); err != nil {
			devdocsfmt.Fatal("%s", err)
		}
	default:
		devdocsfmt.Fatal("unknown --mode %q (use 'overview' or 'release')", *mode)
	}
	fmt.Fprintln(os.Stderr, "wrote", flags.Out)
}
