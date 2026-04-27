package main

import (
	"fmt"

	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

// overviewPages is the curated list of go.dev pages that make up the
// "general" Go documentation slug. Tested via curl on go.dev — every
// path here returns 200. If go.dev removes or renames a page, drop
// or rename the row; one missing page is non-fatal during fetch.
//
// The grouping (Overview / Modules / Database / Tutorial / Reference)
// becomes the Type field in marvindocs search results.
var overviewPages = []struct {
	URL   string // path on go.dev
	Slug  string // db.json key (becomes /docs/<doc>/<slug> in marvindocs)
	Title string
	Type  string
}{
	{"/doc/install", "install", "Installation", "Overview"},
	{"/doc/effective_go", "effective_go", "Effective Go", "Overview"},
	{"/doc/faq", "faq", "Frequently Asked Questions", "Overview"},
	{"/doc/diagnostics.html", "diagnostics", "Diagnostics", "Overview"},
	{"/doc/gc-guide", "gc-guide", "Guide to the Go Garbage Collector", "Overview"},
	{"/doc/comment", "comment", "Go Doc Comments", "Overview"},
	{"/doc/asm", "asm", "A Quick Guide to Go's Assembler", "Reference"},
	{"/doc/cmd", "cmd", "go command", "Reference"},
	{"/doc/build-cover", "build-cover", "Coverage for integration tests", "Reference"},
	{"/doc/pgo", "pgo", "Profile-guided optimization", "Reference"},
	{"/doc/gdb", "gdb", "Debugging Go Code with GDB", "Reference"},
	{"/ref/spec", "spec", "The Go Programming Language Specification", "Reference"},

	{"/doc/modules/developing", "modules/developing", "Developing modules", "Modules"},
	{"/doc/modules/gomod-ref", "modules/gomod-ref", "go.mod file reference", "Modules"},
	{"/doc/modules/layout", "modules/layout", "Module layout", "Modules"},
	{"/doc/modules/major-version", "modules/major-version", "Major version", "Modules"},
	{"/doc/modules/managing-dependencies", "modules/managing-dependencies", "Managing dependencies", "Modules"},
	{"/doc/modules/managing-source", "modules/managing-source", "Managing source", "Modules"},
	{"/doc/modules/publishing", "modules/publishing", "Publishing a module", "Modules"},
	{"/doc/modules/release-workflow", "modules/release-workflow", "Module release workflow", "Modules"},
	{"/doc/modules/version-numbers", "modules/version-numbers", "Module version numbering", "Modules"},

	{"/doc/database/index", "database/index", "Accessing relational databases", "Database"},
	{"/doc/database/open-handle", "database/open-handle", "Opening a database handle", "Database"},
	{"/doc/database/manage-connections", "database/manage-connections", "Managing connections", "Database"},
	{"/doc/database/querying", "database/querying", "Querying for data", "Database"},
	{"/doc/database/change-data", "database/change-data", "Executing SQL statements", "Database"},
	{"/doc/database/prepared-statements", "database/prepared-statements", "Using prepared statements", "Database"},
	{"/doc/database/execute-transactions", "database/execute-transactions", "Executing transactions", "Database"},
	{"/doc/database/cancel-operations", "database/cancel-operations", "Cancelling in-progress operations", "Database"},
	{"/doc/database/sql-injection", "database/sql-injection", "Avoiding SQL injection risk", "Database"},

	{"/doc/tutorial/getting-started", "tutorial/getting-started", "Tutorial: Getting started", "Tutorial"},
	{"/doc/tutorial/create-module.html", "tutorial/create-module", "Tutorial: Create a Go module", "Tutorial"},
	{"/doc/tutorial/database-access", "tutorial/database-access", "Tutorial: Accessing a relational database", "Tutorial"},
	{"/doc/tutorial/web-service-gin", "tutorial/web-service-gin", "Tutorial: Developing a RESTful API with Go and Gin", "Tutorial"},
	{"/doc/tutorial/govet", "tutorial/govet", "Tutorial: Find and fix vulnerable dependencies", "Tutorial"},
}

// runOverview parses every go.dev/doc page in overviewPages and
// emits a single 'go-dev' doc.
func runOverview(flags devdocsfmt.Flags) error {
	w := devdocsfmt.NewWriter(devdocsfmt.Doc{
		Name:    "Go (go.dev)",
		Slug:    "go-dev",
		Type:    "go",
		Version: "",
		Release: "",
		Links: map[string]string{
			"home": "https://go.dev",
			"code": "https://github.com/golang/go",
		},
		Attribution: `&copy; The Go Authors. Licensed under the BSD 3-Clause License.`,
		Mtime:       nowUnix(),
	})

	for _, p := range overviewPages {
		if err := addPage(w, flags.Source, p.URL, p.Slug, p.Title, p.Type); err != nil {
			fmt.Fprintf(stderr, "  ! skip %s: %v\n", p.URL, err)
			continue
		}
	}
	return w.Flush(flags.Out, flags.Force)
}
