package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

// run drives the full parser pipeline:
//  1. find the source/ tree SHA for the requested manual version
//  2. recursively list every .txt file (those are reST sources)
//  3. download each file, render minimal reST -> HTML
//  4. emit the DevDocs tree
func run(flags devdocsfmt.Flags) error {
	short := shortVersion(flags.Version)
	w := devdocsfmt.NewWriter(devdocsfmt.Doc{
		Name:    "MongoDB Manual",
		Slug:    "mongodb~" + short,
		Type:    "mongodb",
		Version: short,
		Release: flags.Release,
		Links: map[string]string{
			"home": "https://www.mongodb.com/docs/manual/",
			"code": "https://github.com/mongodb/docs",
		},
		Attribution: `&copy; MongoDB, Inc. Licensed under <a href="https://creativecommons.org/licenses/by-nc-sa/3.0/us/">CC BY-NC-SA 3.0 US</a>.`,
		Mtime:       time.Now().Unix(),
	})

	tree, err := listSourceTree(flags.Version)
	if err != nil {
		return fmt.Errorf("list source tree: %w", err)
	}

	pages := make([]page, 0, len(tree))
	for _, path := range tree {
		body, err := fetchPage(flags.Source, flags.Version, path)
		if err != nil {
			fmt.Fprintf(stderrW, "  skip %s: %v\n", path, err)
			continue
		}
		slug := stripExt(path)
		title, html, headings := rstToHTML(body)
		if title == "" {
			title = humanize(slug)
		}
		section := topSection(slug)
		w.AddPage(slug, html)
		w.AddEntry(title, slug, section)
		for _, h := range headings {
			if h.id == "" || h.text == "" {
				continue
			}
			w.AddEntry(h.text, slug+"#"+h.id, section)
		}
		pages = append(pages, page{Title: title, Slug: slug, Section: section})
	}
	if len(pages) == 0 {
		return fmt.Errorf("no pages parsed")
	}

	sort.Slice(pages, func(i, j int) bool {
		if pages[i].Section != pages[j].Section {
			return pages[i].Section < pages[j].Section
		}
		return pages[i].Title < pages[j].Title
	})
	w.AddPage("index", buildIndexPage(w.Doc.Name, w.Doc.Version, pages))
	w.AddEntry("Index", "index", "Index")

	return w.Flush(flags.Out, flags.Force)
}

type page struct {
	Title   string
	Slug    string
	Section string
}

// listSourceTree returns every .txt path under the manual's source/
// directory. We resolve the tree SHA via the contents API (which gives
// us the SHA of the source/ subtree) and then list it recursively.
func listSourceTree(version string) ([]string, error) {
	api := "https://api.github.com/repos/mongodb/docs"
	contentsURL := fmt.Sprintf("%s/contents/content/manual/%s?ref=main", api, version)
	body, err := httpGet(contentsURL)
	if err != nil {
		return nil, err
	}
	var entries []struct {
		Name string `json:"name"`
		SHA  string `json:"sha"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(body), &entries); err != nil {
		return nil, fmt.Errorf("parse contents: %w (body=%s)", err, snippet(body))
	}
	var sha string
	for _, e := range entries {
		if e.Name == "source" && e.Type == "dir" {
			sha = e.SHA
			break
		}
	}
	if sha == "" {
		return nil, fmt.Errorf("source/ directory not found for version %s", version)
	}
	treeURL := fmt.Sprintf("%s/git/trees/%s?recursive=1", api, sha)
	body, err = httpGet(treeURL)
	if err != nil {
		return nil, err
	}
	var tree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(body), &tree); err != nil {
		return nil, fmt.Errorf("parse tree: %w", err)
	}
	if tree.Truncated {
		return nil, fmt.Errorf("git tree response was truncated")
	}
	out := make([]string, 0, len(tree.Tree))
	for _, t := range tree.Tree {
		if t.Type != "blob" {
			continue
		}
		if !strings.HasSuffix(t.Path, ".txt") {
			continue
		}
		// Skip include/snippet fragments — they're meant to be transcluded.
		if strings.HasPrefix(t.Path, "includes/") {
			continue
		}
		out = append(out, t.Path)
	}
	sort.Strings(out)
	return out, nil
}

func fetchPage(base, version, path string) (string, error) {
	u := fmt.Sprintf("%s/%s/source/%s", base, version, path)
	return httpGet(u)
}

func httpGet(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	req, _ := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("User-Agent", "marvindocs-mongodb-parser")
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
		return "", fmt.Errorf("GET %s: status %d", rawURL, resp.StatusCode)
	}
	return string(body), nil
}

func snippet(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

// shortVersion strips a leading "v" so "v8.0" → "8.0".
func shortVersion(v string) string {
	return strings.TrimPrefix(v, "v")
}

func stripExt(p string) string {
	return strings.TrimSuffix(p, ".txt")
}

// topSection returns the first path component, e.g. "core" for
// "core/transactions". Single-segment paths get "Reference".
func topSection(slug string) string {
	if i := strings.Index(slug, "/"); i > 0 {
		return humanize(slug[:i])
	}
	return "Manual"
}

var humanizeRe = regexp.MustCompile(`[-_]+`)

func humanize(s string) string {
	s = humanizeRe.ReplaceAllString(s, " ")
	if s == "" {
		return s
	}
	parts := strings.Fields(s)
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func buildIndexPage(name, version string, pages []page) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<h1>%s</h1>\n", htmlEscape(name))
	if version != "" {
		fmt.Fprintf(&b, "<p class=\"_attribution-p\">Version %s</p>\n", htmlEscape(version))
	}
	current := ""
	listOpen := false
	for _, p := range pages {
		if p.Section != current {
			if listOpen {
				b.WriteString("</ul>\n")
				listOpen = false
			}
			fmt.Fprintf(&b, "<h2>%s</h2>\n<ul>\n", htmlEscape(p.Section))
			listOpen = true
			current = p.Section
		}
		fmt.Fprintf(&b, "  <li><a href=\"%s\">%s</a></li>\n",
			htmlEscape(p.Slug), htmlEscape(p.Title))
	}
	if listOpen {
		b.WriteString("</ul>\n")
	}
	return b.String()
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

var stderrW io.Writer = os.Stderr
