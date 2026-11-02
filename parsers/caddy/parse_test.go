package main

import (
	"strings"
	"testing"
)

func TestCleanMarkdown(t *testing.T) {
	in := "<script>\nready(function(){})\n</script>\n\n{{include \"/includes/docs/details.html\"}}\n" +
		"a {{`{{`}}now{{`}}`}} b\n" +
		"c `{{printf \"{{ }}\"}}`\n" +
		"d \"{{printf \"I'm {{.N}}\"}}\"\n" +
		"e `{{ \"{{placeholder \\\"x\\\"}}\" }}`\n"
	want := "a {{now}} b\nc `{{ }}`\nd \"I'm {{.N}}\"\ne `{{placeholder \"x\"}}`\n"
	if got := cleanMarkdown(in); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestSplitFrontMatter(t *testing.T) {
	title, body := splitFrontMatter("---\ntitle: reverse_proxy (Caddyfile directive)\n---\n\n# reverse_proxy\n")
	if title != "reverse_proxy (Caddyfile directive)" || !strings.Contains(body, "# reverse_proxy") {
		t.Fatalf("title=%q body=%q", title, body)
	}
	if shortTitle(title) != "reverse_proxy" {
		t.Fatalf("shortTitle=%q", shortTitle(title))
	}
}

func TestRewriteLinks(t *testing.T) {
	pages := map[string]string{"conventions": "x", "caddyfile/directives/root": "x", "caddyfile/directives/file_server": "x"}
	in := `<a href="/docs/conventions#durations">d</a> <a href="root">r</a> <a href="/docs/json/">j</a> <a href="/download">dl</a> <a href="#x">x</a> <img src="/old/x.png">`
	want := `<a href="../../conventions#durations">d</a> <a href="root">r</a> <a href="https://caddyserver.com/docs/json/">j</a> <a href="https://caddyserver.com/download">dl</a> <a href="#x">x</a> <img src="https://caddyserver.com/old/x.png">`
	if got := rewriteLinks(in, "caddyfile/directives/file_server", pages); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestExtractHeadings(t *testing.T) {
	body := mdToHTML("# T\n\n## Syntax\n\n### The `to` option &amp; more\n\n#### deep\n")
	hs := extractHeadings(body)
	if len(hs) != 2 || hs[0] != (heading{"Syntax", "syntax"}) || hs[1].text != "The to option & more" {
		t.Fatalf("headings: %+v", hs)
	}
}

func TestNavAndSection(t *testing.T) {
	nav := parseNav(`<nav><ul><li class="heading">Reference</li><li><a href="/docs/caddyfile">C</a></li></ul>
<ul><li class="heading">Developers</li><li><a href="/docs/extending-caddy">E</a></li></ul>
<!-- <ul><li class="heading">Old</li><li><a href="/docs/basics">B</a></li></ul> --></nav>`)
	cases := map[string]string{
		"caddyfile":                          "Reference",
		"caddyfile/options":                  "Reference",
		"caddyfile/directives/reverse_proxy": "Caddyfile directives",
		"extending-caddy/namespaces":         "Developers",
		"basics":                             "Other",
	}
	for p, want := range cases {
		if got := sectionFor(p, nav); got != want {
			t.Errorf("%s: got %q want %q", p, got, want)
		}
	}
}
