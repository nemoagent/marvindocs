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

// TestParseTOCLegacy covers the pre-5.0 TOC: bare "- Section" lines
// instead of "- ## Section" and a literal version number in URLs
// instead of "{{version}}". The parser must accept both shapes.
func TestParseTOCLegacy(t *testing.T) {
	md := `- Preface
    - [Introduction](/docs/4.0/introduction)
    - [Quickstart](/docs/4.0/quick)
- Getting Started
    - [Installation](/docs/4.0/installation)
    - [Routing](/docs/4.0/routing)
`
	pages := parseTOC(md)
	if len(pages) != 4 {
		t.Fatalf("want 4 pages, got %d (%+v)", len(pages), pages)
	}
	if pages[0].Slug != "introduction" || pages[0].Section != "Preface" {
		t.Fatalf("first page wrong: %+v", pages[0])
	}
	if pages[3].Slug != "routing" || pages[3].Section != "Getting Started" {
		t.Fatalf("routing wrong: %+v", pages[3])
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
	cases := map[string]string{
		"12.x":   "12",
		"6.x":    "6",
		"master": "master",
		"4.0":    "4.0",
		"4.1":    "4.1",
		"5.1":    "5.1",
		"5.8":    "5.8",
	}
	for in, want := range cases {
		if got := shortVersion(in); got != want {
			t.Errorf("shortVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeVersion(t *testing.T) {
	cases := map[string]string{
		"12":     "12.x",
		"6":      "6.x",
		"12.x":   "12.x",
		"5.1":    "5.1",
		"4.0":    "4.0",
		"master": "master",
		"":       "",
	}
	for in, want := range cases {
		if got := normalizeVersion(in); got != want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", in, got, want)
		}
	}
}
