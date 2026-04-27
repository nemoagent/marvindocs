// Command mongodb-parser fetches the MongoDB Manual reST sources from
// the mongodb/docs GitHub repository and emits a DevDocs-shaped tree
// (meta.json/index.json/db.json) that marvindocs can index.
//
// Usage:
//
//	mongodb-parser --version v8.0 --out data/public/docs/mongodb~8.0
//
// Source defaults to the official repo on GitHub. Network access is
// required: the parser hits raw.githubusercontent.com to download the
// table of contents and every linked page.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

func main() {
	fs := flag.NewFlagSet("mongodb-parser", flag.ExitOnError)
	flags := devdocsfmt.Flags{}
	flags.RegisterFlags(fs)
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if flags.Source == "" {
		flags.Source = "https://raw.githubusercontent.com/mongodb/docs/main/content/manual"
	}
	if flags.Version == "" {
		flags.Version = "v8.0"
	}
	if err := flags.Validate(); err != nil {
		devdocsfmt.Fatal("%s", err)
	}
	if err := run(flags); err != nil {
		devdocsfmt.Fatal("%s", err)
	}
	fmt.Fprintln(os.Stderr, "wrote", flags.Out)
}
