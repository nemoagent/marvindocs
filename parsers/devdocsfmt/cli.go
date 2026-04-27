package devdocsfmt

import (
	"flag"
	"fmt"
	"os"
)

// Flags is the standard set of CLI options every parser accepts.
// Parsers are free to add their own with fs.* but should keep these
// names so users have a uniform experience across parsers.
type Flags struct {
	Source  string // upstream URL or local path
	Out     string // output directory (data/public/docs/<slug>)
	Version string
	Release string
	Force   bool // overwrite existing output
}

// RegisterFlags wires the standard fields into fs. The parser-specific
// main() typically calls flag.Parse() (or fs.Parse(os.Args[1:])) after
// adding any extra flags.
func (f *Flags) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(&f.Source, "source", "", "Upstream source (URL, repo path, etc.) — parser-specific")
	fs.StringVar(&f.Out, "out", "", "Output directory, e.g. data/public/docs/laravel~12")
	fs.StringVar(&f.Version, "version", "", "Version label written to meta.json")
	fs.StringVar(&f.Release, "release", "", "Release label written to meta.json (defaults to version)")
	fs.BoolVar(&f.Force, "force", false, "Overwrite existing files in --out")
}

// Validate checks that the mandatory flags are set. Source is treated
// as required because every real parser needs to know what to read.
func (f *Flags) Validate() error {
	if f.Source == "" {
		return fmt.Errorf("--source is required")
	}
	if f.Out == "" {
		return fmt.Errorf("--out is required")
	}
	if f.Release == "" {
		f.Release = f.Version
	}
	return nil
}

// Fatal prints "error: <msg>" to stderr and exits with code 1.
// Convenience wrapper to keep parser main()s short.
func Fatal(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+msg+"\n", args...)
	os.Exit(1)
}
