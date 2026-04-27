package main

import (
	"strings"
	"testing"
)

func TestParseTOC(t *testing.T) {
	md := `# Documentation

- ## Prologue
    - [Release Notes](/docs/{{version}}/releases)
    - [Upgrade Guide](/docs/{{version}}/upgrade)
- ## The Basics
    - [Routing](/docs/{{version}}/routing)
    - [Middleware](/docs/{{version}}/middleware)
`
	pages := parseTOC(md)
	if len(pages) != 4 {
		t.Fatalf("want 4 pages, got %d", len(pages))
	}
	if pages[0].Slug != "releases" || pages[0].Section != "Prologue" {
		t.Fatalf("first page wrong: %+v", pages[0])
	}
	if pages[2].Slug != "routing" || pages[2].Section != "The Basics" {
		t.Fatalf("routing wrong: %+v", pages[2])
	}
}

func TestExtractHeadings(t *testing.T) {
	md := "# Top\n\n## Basic Routing\n\n### The Default Route Files\n\nbody\n\n## Route Parameters\n"
	hs := extractHeadings(md)
	if len(hs) != 3 {
		t.Fatalf("want 3 headings, got %d (%+v)", len(hs), hs)
	}
	if hs[0].id != "basic-routing" || hs[0].text != "Basic Routing" {
		t.Errorf("first heading: %+v", hs[0])
	}
	if hs[1].id != "the-default-route-files" {
		t.Errorf("second id: %s", hs[1].id)
	}
}

func TestAnchorID(t *testing.T) {
	cases := map[string]string{
		"Basic Routing":                  "basic-routing",
		"Route Model Binding (Advanced)": "route-model-binding-advanced",
		"   Padded   ":                   "padded",
		"123 numbers":                    "123-numbers",
	}
	for in, want := range cases {
		got := anchorID(strings.TrimSpace(in))
		if got != want {
			t.Errorf("anchorID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShortVersion(t *testing.T) {
	if shortVersion("12.x") != "12" {
		t.Errorf("12.x")
	}
	if shortVersion("master") != "master" {
		t.Errorf("master")
	}
	if shortVersion("11.x") != "11" {
		t.Errorf("11.x")
	}
}
