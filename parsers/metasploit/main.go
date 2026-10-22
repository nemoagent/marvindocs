package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

func main() {
	var f devdocsfmt.Flags
	f.RegisterFlags(flag.CommandLine)
	flag.Parse()
	if f.Source == "" {
		f.Source = "https://docs.metasploit.com/"
	}
	if err := f.Validate(); err != nil {
		devdocsfmt.Fatal("%s", err)
	}
	if err := run(f); err != nil {
		devdocsfmt.Fatal("%s", err)
	}
	fmt.Fprintln(os.Stderr, "wrote", f.Out)
}
