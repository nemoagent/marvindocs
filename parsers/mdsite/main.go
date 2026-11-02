// Command mdsite-parser converts Markdown documentation sites (Hugo,
// mkdocs, Docusaurus) kept in git repositories into DevDocs-shaped
// trees. Every supported site is described in sites*.go.
//
// Usage:
//
//	mdsite-parser --site docker --out data/public/docs/docker --force
//	mdsite-parser --list
//
// Sources are fetched with a shallow sparse git clone of the doc
// directories only. --source overrides the clone with a local checkout
// (single-source sites only).
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

func main() {
	fs := flag.NewFlagSet("mdsite-parser", flag.ExitOnError)
	flags := devdocsfmt.Flags{}
	flags.RegisterFlags(fs)
	name := fs.String("site", "", "site to parse (see --list)")
	list := fs.Bool("list", false, "list supported sites")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if *list {
		names := make([]string, 0, len(sites))
		for n := range sites {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Printf("%-16s %s\n", n, sites[n].Name)
		}
		return
	}
	site, ok := sites[*name]
	if !ok {
		devdocsfmt.Fatal("unknown --site %q (see --list)", *name)
	}
	if flags.Out == "" {
		devdocsfmt.Fatal("--out is required")
	}
	if flags.Release == "" {
		flags.Release = flags.Version
	}
	if err := run(site, flags); err != nil {
		devdocsfmt.Fatal("%s", err)
	}
	fmt.Fprintln(os.Stderr, "wrote", flags.Out)
}
