// Command laravel-parser fetches the Laravel docs markdown source
// from GitHub and emits a DevDocs-shaped tree (meta.json/index.json/
// db.json) that marvindocs can index without modification.
//
// Usage:
//
//	laravel-parser --version 12.x --out data/public/docs/laravel~12
//
// The --source flag is optional; it defaults to the official repo.
// Network access is required: the parser hits raw.githubusercontent.com
// to download both the table of contents and every linked page.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

func main() {
	fs := flag.NewFlagSet("laravel-parser", flag.ExitOnError)
	flags := devdocsfmt.Flags{}
	flags.RegisterFlags(fs)

	// Override defaults for the Laravel-specific flow.
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if flags.Source == "" {
		flags.Source = "https://raw.githubusercontent.com/laravel/docs"
	}
	if flags.Version == "" {
		flags.Version = "12.x"
	}
	flags.Version = normalizeVersion(flags.Version)
	if err := flags.Validate(); err != nil {
		devdocsfmt.Fatal("%s", err)
	}

	if err := run(flags); err != nil {
		devdocsfmt.Fatal("%s", err)
	}
	fmt.Fprintln(os.Stderr, "wrote", flags.Out)
}
