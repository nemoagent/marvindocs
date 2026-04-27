// Command go-std-parser collects standard-library docs from `go doc`
// inside a Docker image and emits a DevDocs-shaped tree.
//
// Usage:
//
//	go-std-parser --version 1.25 --out data/public/docs/go-std~1.25
//
// The parser does not fetch docs from the internet; it executes
// `docker run <image> go doc ...` against the selected Go image.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

func main() {
	fs := flag.NewFlagSet("go-std-parser", flag.ExitOnError)
	flags := devdocsfmt.Flags{}
	flags.RegisterFlags(fs)
	image := fs.String("image", "", "Docker image with Go toolchain (default: golang:<version>)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}

	if flags.Version == "" {
		flags.Version = "1.25"
	}
	if flags.Release == "" {
		flags.Release = flags.Version
	}
	if *image == "" {
		*image = "golang:" + flags.Version
	}
	if flags.Out == "" {
		flags.Out = "data/public/docs/go-std~" + flags.Version
	}
	if flags.Source == "" {
		flags.Source = "docker://" + *image
	}

	if err := run(flags, *image); err != nil {
		devdocsfmt.Fatal("%s", err)
	}
	fmt.Fprintln(os.Stderr, "wrote", flags.Out)
}
