// Command caddy-parser converts the Caddy documentation (Markdown
// sources of github.com/caddyserver/website) into a DevDocs-shaped tree.
//
// Usage:
//
//	caddy-parser --out data/public/docs/caddy --force
//
// --source is either the repository tarball URL (default) or a local
// checkout of caddyserver/website.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

const defaultSource = "https://codeload.github.com/caddyserver/website/tar.gz/refs/heads/master"

func main() {
	fs := flag.NewFlagSet("caddy-parser", flag.ExitOnError)
	flags := devdocsfmt.Flags{}
	flags.RegisterFlags(fs)
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if flags.Source == "" {
		flags.Source = defaultSource
	}
	if err := flags.Validate(); err != nil {
		devdocsfmt.Fatal("%s", err)
	}
	if err := run(flags); err != nil {
		devdocsfmt.Fatal("%s", err)
	}
	fmt.Fprintln(os.Stderr, "wrote", flags.Out)
}
