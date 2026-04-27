package main

import (
	"fmt"
	"strings"

	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

// runRelease parses the release-notes page for one specific Go
// version (e.g. "1.22") and emits a per-version doc.
//
// go.dev publishes release notes at /doc/go1.22, /doc/go1.21, etc.
// We treat each h2 ("Tools", "Standard library") as a section
// (Type) and each h3/h4 inside as an entry under that section.
func runRelease(flags devdocsfmt.Flags) error {
	url := flags.Source + "/doc/go" + flags.Version
	body, headings, err := fetchArticle(url)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", url, err)
	}

	w := devdocsfmt.NewWriter(devdocsfmt.Doc{
		Name:    "Go " + flags.Version + " release notes",
		Slug:    "go-dev~" + flags.Version,
		Type:    "go",
		Version: flags.Version,
		Release: flags.Release,
		Links: map[string]string{
			"home": "https://go.dev",
			"code": "https://github.com/golang/go",
		},
		Attribution: `&copy; The Go Authors. Licensed under the BSD 3-Clause License.`,
		Mtime:       nowUnix(),
	})

	// Single page; everything lives at "release".
	w.AddPage("release", body)
	w.AddEntry("Go "+flags.Version+" Release Notes", "release", "Overview")

	// Use the most recent h2 as the section for any h3/h4 hits.
	currentSection := "Overview"
	for _, h := range headings {
		if h.text == "" {
			continue
		}
		if h.level == 2 {
			currentSection = strings.TrimSpace(h.text)
			w.AddEntry(h.text, "release#"+h.id, currentSection)
			continue
		}
		w.AddEntry(h.text, "release#"+h.id, currentSection)
	}

	return w.Flush(flags.Out, flags.Force)
}
