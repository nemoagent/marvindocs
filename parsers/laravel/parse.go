package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gomarkdown/markdown"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

// run drives the full parser pipeline: fetch TOC, fetch each linked
// page, convert markdown to HTML, register entries from headings,
// and flush the DevDocs tree.
func run(flags devdocsfmt.Flags) error {
	w := devdocsfmt.NewWriter(devdocsfmt.Doc{
		Name:    "Laravel",
		Slug:    "laravel~" + shortVersion(flags.Version),
		Type:    "laravel",
		Version: flags.Version,
		Release: flags.Release,
		Links: map[string]string{
			"home": "https://laravel.com",
			"code": "https://github.com/laravel/laravel",
		},
		Attribution: `&copy; Taylor Otwell. Licensed under the MIT License.`,
		Mtime:       time.Now().Unix(),
	})

	toc, err := fetchTOC(flags.Source, flags.Version)
	if err != nil {
		return fmt.Errorf("fetch TOC: %w", err)
	}
	pages := parseTOC(toc)
	if len(pages) == 0 {
		return fmt.Errorf("TOC parse returned 0 pages")
	}

	for _, p := range pages {
		md, err := fetchPage(flags.Source, flags.Version, p.Slug)
		if err != nil {
			fmt.Fprintf(stderr, "  skip %s: %v\n", p.Slug, err)
			continue
		}
		htmlBody := mdToHTML(md)
		w.AddPage(p.Slug, htmlBody)
		// Top-level entry for the page itself.
		w.AddEntry(p.Title, p.Slug, p.Section)
		// Sub-entries for every H2/H3 heading on the page.
		for _, h := range extractHeadings(md) {
			w.AddEntry(h.text, p.Slug+"#"+h.id, p.Section)
		}
	}

	// Marvindocs (and DevDocs) expect an "index" page that web UI
	// hits when you open /docs/<slug>/. Build one from the TOC so
	// users land on a real landing page instead of a 404.
	w.AddPage("index", buildIndexPage(w.Doc.Name, w.Doc.Version, pages))

	return w.Flush(flags.Out, flags.Force)
}

// buildIndexPage renders a static landing page that mirrors the
// upstream documentation.md TOC: one section per "## Section",
// links to every page in the section. Generated server-side once,
// no JS, plain HTML compatible with marvindocs' page renderer.
func buildIndexPage(name, version string, pages []page) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<h1>%s</h1>\n", htmlEscape(name))
	if version != "" {
		fmt.Fprintf(&b, "<p class=\"_attribution-p\">Version %s</p>\n", htmlEscape(version))
	}

	currentSection := ""
	listOpen := false
	for _, p := range pages {
		if p.Section != currentSection {
			if listOpen {
				b.WriteString("</ul>\n")
				listOpen = false
			}
			fmt.Fprintf(&b, "<h2>%s</h2>\n<ul>\n", htmlEscape(p.Section))
			listOpen = true
			currentSection = p.Section
		}
		fmt.Fprintf(&b, "  <li><a href=\"%s\">%s</a></li>\n",
			htmlEscape(p.Slug), htmlEscape(p.Title))
	}
	if listOpen {
		b.WriteString("</ul>\n")
	}
	return b.String()
}

// htmlEscape is a minimal escape for the small set of characters we
// can encounter in TOC titles and slugs. Stays inline so we don't
// add a dependency on text/template just for this.
func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// stderr is a package-level alias so run() can be redirected in
// tests; main.go writes to os.Stderr through it.
var stderr io.Writer = os.Stderr

// page is one row from the TOC: a title shown in the sidebar, the
// markdown filename it points to, and the section it belongs to.
type page struct {
	Title   string
	Slug    string
	Section string
}

// fetchTOC downloads documentation.md from the laravel/docs repo.
// The TOC lists every page in the doc set under "## Section" headings.
func fetchTOC(base, version string) (string, error) {
	return httpGet(fmt.Sprintf("%s/%s/documentation.md", base, version))
}

// fetchPage downloads <slug>.md.
func fetchPage(base, version, slug string) (string, error) {
	return httpGet(fmt.Sprintf("%s/%s/%s.md", base, version, slug))
}

// httpGet does the boring HTTP work. Any non-200 status is an error.
func httpGet(url string) (string, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "marvindocs-laravel-parser")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return string(body), nil
}

// parseTOC walks documentation.md (a Markdown bullet list) and pulls
// out one page per "[Title](/docs/<version-or-placeholder>/<slug>)"
// link, remembering the most recent section heading so each page
// knows where it belongs.
//
// The TOC format has changed across Laravel releases:
//
//   - Old (4.x): "- Section" without "##", links use literal version
//     ("/docs/4.0/routing").
//   - New (5.x+): "- ## Section", links use placeholder
//     ("/docs/{{version}}/routing").
//
// We accept both. Section is matched as either "- ## NAME" or
// "- NAME" at the start of a line (top-level bullet). Page links
// use a permissive path component so any version literal works.
func parseTOC(md string) []page {
	sectionRe := regexp.MustCompile(`(?m)^- (?:## )?(.+)$`)
	linkRe := regexp.MustCompile(`\[([^\]]+)\]\(/docs/[^/)]+/([^)#]+)\)`)

	var pages []page
	currentSection := ""
	for _, line := range strings.Split(md, "\n") {
		// A top-level "- ..." line that has no link is a section header.
		// A line with a link is a page entry.
		hasLink := linkRe.MatchString(line)
		if m := sectionRe.FindStringSubmatch(line); m != nil && !hasLink {
			currentSection = strings.TrimSpace(m[1])
			continue
		}
		if m := linkRe.FindStringSubmatch(line); m != nil {
			pages = append(pages, page{
				Title:   strings.TrimSpace(m[1]),
				Slug:    strings.TrimSpace(m[2]),
				Section: currentSection,
			})
		}
	}
	return pages
}

// mdToHTML converts a markdown body into an HTML fragment. The
// gomarkdown renderer with HrefTargetBlank=false keeps internal links
// as-is, which is what marvindocs needs (link rewriting happens at
// render time).
func mdToHTML(md string) string {
	exts := parser.CommonExtensions | parser.AutoHeadingIDs
	p := parser.NewWithExtensions(exts)
	doc := p.Parse([]byte(md))

	opts := mdhtml.RendererOptions{
		Flags: mdhtml.CommonFlags &^ mdhtml.UseXHTML,
	}
	r := mdhtml.NewRenderer(opts)
	return string(markdown.Render(doc, r))
}

// heading is one H2/H3 we plan to expose as a search entry.
type heading struct {
	text string
	id   string
}

var headingRe = regexp.MustCompile(`(?m)^(#{2,3})\s+(.+?)\s*$`)

// extractHeadings finds H2/H3 headings in the source markdown and
// returns the text plus the auto-generated anchor id (matching the
// id gomarkdown will emit). Only headings deeper than H1 are useful;
// H1 is already covered by the page-level entry.
func extractHeadings(md string) []heading {
	var hs []heading
	for _, m := range headingRe.FindAllStringSubmatch(md, -1) {
		text := strings.TrimSpace(m[2])
		// Strip any inline backticks/markdown around the heading text;
		// the rendered HTML will have them but we want the cleaner
		// version in search results.
		clean := strings.ReplaceAll(text, "`", "")
		hs = append(hs, heading{text: clean, id: anchorID(text)})
	}
	return hs
}

// anchorID is a port of gomarkdown's AutoHeadingIDs algorithm: lower
// case, replace spaces with dashes, strip non-alphanumerics. Kept
// inline so we don't have to import an internal package.
var nonAlnumDash = regexp.MustCompile(`[^a-z0-9-]+`)

func anchorID(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlnumDash.ReplaceAllString(s, "")
	return strings.Trim(s, "-")
}

// shortVersion derives the slug suffix from a Laravel branch name.
//
// Branches like "12.x" become "12" — the ".x" is just a placeholder
// for "current minor" so we drop it. Older Laravel releases use real
// dotted versions like "4.0", "5.1" — keep those intact, otherwise
// every minor would collide on a single "<major>" slug.
//
// Special branches like "master" pass through unchanged.
func shortVersion(v string) string {
	if strings.HasSuffix(v, ".x") {
		return strings.TrimSuffix(v, ".x")
	}
	return v
}

// normalizeVersion maps a bare major ("12") to its branch name ("12.x").
// Dotted versions ("5.1", "12.x") and "master" stay as-is.
func normalizeVersion(v string) string {
	if v == "" {
		return v
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return v
		}
	}
	return v + ".x"
}
