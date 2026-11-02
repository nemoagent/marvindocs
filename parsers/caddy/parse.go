package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gomarkdown/markdown"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

const (
	siteURL = "https://caddyserver.com"
	mdDir   = "src/docs/markdown/"
	navFile = "src/docs/index.html"
)

func run(flags devdocsfmt.Flags) error {
	files, err := loadSource(flags.Source)
	if err != nil {
		return fmt.Errorf("load source: %w", err)
	}
	pages := map[string]string{} // page path -> markdown
	for name, body := range files {
		if strings.HasPrefix(name, mdDir) && strings.HasSuffix(name, ".md") {
			pages[strings.TrimSuffix(strings.TrimPrefix(name, mdDir), ".md")] = body
		}
	}
	if len(pages) == 0 {
		return fmt.Errorf("no markdown under %s", mdDir)
	}
	// Filled by JavaScript from the live module registry; links to them
	// stay absolute to caddyserver.com.
	delete(pages, "json")
	delete(pages, "modules")
	nav := parseNav(files[navFile])

	w := devdocsfmt.NewWriter(devdocsfmt.Doc{
		Name:    "Caddy",
		Slug:    "caddy",
		Type:    "caddy",
		Version: flags.Version,
		Release: flags.Release,
		Links: map[string]string{
			"home": siteURL,
			"code": "https://github.com/caddyserver/caddy",
		},
		Attribution: `&copy; Matthew Holt and The Caddy Authors. Documentation from <a href="https://github.com/caddyserver/website">caddyserver/website</a>, licensed under the Apache License 2.0.`,
		Mtime:       time.Now().Unix(),
	})

	names := make([]string, 0, len(pages))
	for p := range pages {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, p := range names {
		title, md := splitFrontMatter(pages[p])
		body := rewriteLinks(mdToHTML(cleanMarkdown(md)), p, pages)
		if title == "" {
			if m := h1Re.FindStringSubmatch(body); m != nil {
				title = strings.TrimSpace(htmlUnescape(tagRe.ReplaceAllString(m[1], "")))
			}
		}
		if title == "" {
			title = p
		}
		section := sectionFor(p, nav)
		w.AddPage(p, body)
		w.AddEntry(title, p, section)
		for _, h := range extractHeadings(body) {
			w.AddEntry(shortTitle(title)+": "+h.text, p+"#"+h.id, section)
		}
	}
	return w.Flush(flags.Out, flags.Force)
}

// loadSource returns repository files (path relative to repo root)
// from a local checkout or a .tar.gz URL of caddyserver/website.
func loadSource(src string) (map[string]string, error) {
	if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
		files := map[string]string{}
		err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(src, p)
			rel = filepath.ToSlash(rel)
			if rel != navFile && !strings.HasPrefix(rel, mdDir) {
				return nil
			}
			b, err := os.ReadFile(p)
			files[rel] = string(b)
			return err
		})
		return files, err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Get(src)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", src, resp.Status)
	}
	return readTarGz(resp.Body)
}

func readTarGz(r io.Reader) (map[string]string, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	files := map[string]string{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return files, nil
		}
		if err != nil {
			return nil, err
		}
		// Strip the "website-master/" top-level directory.
		_, rel, ok := strings.Cut(h.Name, "/")
		if !ok || h.Typeflag != tar.TypeReg || (rel != navFile && !strings.HasPrefix(rel, mdDir)) {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		files[rel] = string(b)
	}
}

var titleRe = regexp.MustCompile(`(?m)^title:\s*"?(.*?)"?\s*$`)

func splitFrontMatter(md string) (title, body string) {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	if !strings.HasPrefix(md, "---\n") {
		return "", md
	}
	fm, rest, ok := strings.Cut(md[4:], "\n---\n")
	if !ok {
		return "", md
	}
	if m := titleRe.FindStringSubmatch(fm); m != nil {
		title = strings.TrimSpace(m[1])
	}
	return title, rest
}

var (
	scriptRe   = regexp.MustCompile(`(?s)<(script|style)\b.*?</(script|style)>\s*`)
	includeRe  = regexp.MustCompile(`[ \t]*\{\{\s*include\s+"[^"]*"\s*\}\}[ \t]*\n?`)
	rawStrRe   = regexp.MustCompile("\\{\\{`(.*?)`\\}\\}")
	printfRe   = regexp.MustCompile(`\{\{\s*(?:printf\s+)?"((?:[^"\\]|\\.)*)"\s*\}\}`)
	headingRe  = regexp.MustCompile(`(?s)<h([23]) id="([^"]+)">(.*?)</h[23]>`)
	h1Re       = regexp.MustCompile(`(?s)<h1[^>]*>(.*?)</h1>`)
	tagRe      = regexp.MustCompile(`<[^>]+>`)
	hrefRe     = regexp.MustCompile(`(href|src)="([^"]*)"`)
	navLinkRe  = regexp.MustCompile(`<li class="heading">([^<]+)</li>|href="/docs/([^"#]*)"`)
	parenTitle = regexp.MustCompile(`\s*\(.*\)$`)
)

// cleanMarkdown drops the site-only scripts and template includes and
// evaluates the Go template escapes the website uses to print literal
// "{{" / "}}".
func cleanMarkdown(md string) string {
	md = scriptRe.ReplaceAllString(md, "")
	md = includeRe.ReplaceAllString(md, "")
	md = rawStrRe.ReplaceAllString(md, "$1")
	return printfRe.ReplaceAllStringFunc(md, func(s string) string {
		v := printfRe.FindStringSubmatch(s)[1]
		return strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(v)
	})
}

func mdToHTML(md string) string {
	p := parser.NewWithExtensions(parser.CommonExtensions | parser.AutoHeadingIDs)
	r := mdhtml.NewRenderer(mdhtml.RendererOptions{Flags: mdhtml.CommonFlags &^ mdhtml.UseXHTML})
	return string(markdown.Render(p.Parse([]byte(md)), r))
}

// rewriteLinks makes /docs/<page> links relative to the current page
// when the page exists in the doc, and every other site-absolute
// link absolute to caddyserver.com.
func rewriteLinks(body, page string, pages map[string]string) string {
	return hrefRe.ReplaceAllStringFunc(body, func(s string) string {
		m := hrefRe.FindStringSubmatch(s)
		attr, u := m[1], m[2]
		if !strings.HasPrefix(u, "/") || strings.HasPrefix(u, "//") {
			return s
		}
		target, frag, _ := strings.Cut(u, "#")
		if p := strings.Trim(strings.TrimPrefix(target, "/docs/"), "/"); attr == "href" && strings.HasPrefix(target, "/docs/") && pages[p] != "" {
			rel, err := filepath.Rel(path.Dir(page), p)
			if err == nil {
				u = filepath.ToSlash(rel)
				if frag != "" {
					u += "#" + frag
				}
				return attr + `="` + u + `"`
			}
		}
		return attr + `="` + siteURL + u + `"`
	})
}

type heading struct{ text, id string }

func extractHeadings(body string) []heading {
	var hs []heading
	for _, m := range headingRe.FindAllStringSubmatch(body, -1) {
		text := strings.Join(strings.Fields(htmlUnescape(tagRe.ReplaceAllString(m[3], ""))), " ")
		if text != "" {
			hs = append(hs, heading{text, m[2]})
		}
	}
	return hs
}

func htmlUnescape(s string) string {
	return strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(s)
}

// shortTitle trims qualifiers like "(Caddyfile directive)" so sub-entries
// read "reverse_proxy: Syntax".
func shortTitle(t string) string {
	return parenTitle.ReplaceAllString(t, "")
}

// parseNav maps page paths to the sidebar heading they appear under.
// Only the first <nav> outside HTML comments is used.
func parseNav(index string) map[string]string {
	if i := strings.Index(index, "<nav>"); i >= 0 {
		index = index[i:]
	}
	if i := strings.Index(index, "<!--"); i >= 0 {
		index = index[:i]
	}
	nav := map[string]string{}
	section := ""
	for _, m := range navLinkRe.FindAllStringSubmatch(index, -1) {
		if m[1] != "" {
			section = strings.TrimSpace(m[1])
		} else if _, ok := nav[m[2]]; !ok && section != "" {
			nav[strings.Trim(m[2], "/")] = section
		}
	}
	return nav
}

// sectionFor picks the entry type: directive pages get their own
// group, other pages inherit the sidebar section of the nearest
// listed ancestor.
func sectionFor(p string, nav map[string]string) string {
	if strings.HasPrefix(p, "caddyfile/directives/") {
		return "Caddyfile directives"
	}
	for q := p; q != "."; q = path.Dir(q) {
		if s, ok := nav[q]; ok {
			return s
		}
	}
	return "Other"
}
