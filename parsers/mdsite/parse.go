package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
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

// Source is one git repository and the directories read from it.
// File keys are Prefix + path inside the repository.
type Source struct {
	Repo   string // clone URL
	Ref    string // branch or tag, "" = default branch
	Dirs   []string
	Prefix string
}

// Root maps Markdown files under Dir (a file key prefix) to pages under
// Page. Page "" puts them at the top of the doc.
type Root struct{ Dir, Page string }

// Mount copies file key Src (a file or a directory) to Dst, like Hugo
// module mounts of vendored docs.
type Mount struct{ Src, Dst string }

// Site describes one documentation set.
type Site struct {
	Name, Slug, Type   string
	Home, Code         string
	Attribution        string
	Sources            []Source
	Roots              []Root
	Mounts             []Mount
	Skip               []string                      // page path prefixes to drop
	IncludeDirs        []string                      // file key dirs searched by include shortcodes
	BaseURL, URLPrefix string                        // page p lives at BaseURL + URLPrefix + p + "/"
	SectionDepth       int                           // path segments that form the entry type (default 1)
	Liquid             bool                          // strip {% ... %} tags
	Section            func(page string) string      // overrides the SectionDepth rule
	Prepare            func(files map[string]string) // edits loaded files before mapping
	Clean              func(md string, files map[string]string) string
	Extra              func(files map[string]string, add func(Page)) error
}

// Page is one converted document before it is written out.
type Page struct {
	Path, Title, Section, HTML string
	file                       string // source file key, "" for generated pages
}

func run(site Site, flags devdocsfmt.Flags) error {
	files, err := load(site, flags.Source)
	if err != nil {
		return fmt.Errorf("load source: %w", err)
	}
	fileToPage := mapPages(site, files)
	if len(fileToPage) == 0 {
		return fmt.Errorf("no markdown found for %s", site.Slug)
	}
	var pages []Page
	for f, p := range fileToPage {
		title, md := splitFrontMatter(files[f])
		body := mdToHTML(cleanMarkdown(site, md, files, 0))
		if title == "" {
			if m := h1Re.FindStringSubmatch(body); m != nil {
				title = plain(m[1])
			}
		} else if !h1Re.MatchString(body) {
			body = "<h1>" + htmlEscape(title) + "</h1>\n" + body
		}
		if title == "" {
			title = humanize(path.Base(p))
		}
		pages = append(pages, Page{Path: p, Title: title, HTML: body, file: f})
	}
	titles := map[string]string{}
	for _, pg := range pages {
		titles[pg.Path] = pg.Title
	}
	for i := range pages {
		pages[i].Section = sectionFor(site, pages[i].Path, titles)
	}
	if site.Extra != nil {
		if err := site.Extra(files, func(p Page) { pages = append(pages, p) }); err != nil {
			return err
		}
	}
	known := map[string]bool{}
	for _, pg := range pages {
		known[pg.Path] = true
	}
	for i, pg := range pages {
		pages[i].HTML = rewriteLinks(site, pg.HTML, pg.file, pg.Path, fileToPage, known)
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Path < pages[j].Path })

	w := devdocsfmt.NewWriter(devdocsfmt.Doc{
		Name:        site.Name,
		Slug:        site.Slug,
		Type:        site.Type,
		Version:     flags.Version,
		Release:     flags.Release,
		Links:       map[string]string{"home": site.Home, "code": site.Code},
		Attribution: site.Attribution,
		Mtime:       time.Now().Unix(),
	})
	for _, pg := range pages {
		w.AddPage(pg.Path, pg.HTML)
		w.AddEntry(pg.Title, pg.Path, pg.Section)
		short := parenTitle.ReplaceAllString(pg.Title, "")
		for _, h := range extractHeadings(pg.HTML) {
			w.AddEntry(short+": "+h.text, pg.Path+"#"+h.id, pg.Section)
		}
	}
	fmt.Fprintf(os.Stderr, "%s: %d pages\n", site.Slug, len(pages))
	return w.Flush(flags.Out, flags.Force)
}

// load reads every text file under the source dirs. src, when set, is a
// local checkout used instead of cloning (single-source sites only).
func load(site Site, src string) (map[string]string, error) {
	files := map[string]string{}
	for _, s := range site.Sources {
		dir := src
		if dir == "" {
			tmp, err := os.MkdirTemp("", "mdsite-")
			if err != nil {
				return nil, err
			}
			defer os.RemoveAll(tmp)
			if err := clone(s, tmp); err != nil {
				return nil, err
			}
			dir = tmp
		} else if len(site.Sources) > 1 {
			return nil, fmt.Errorf("--source is not supported for multi-repo site %s", site.Slug)
		}
		for _, d := range s.Dirs {
			if err := readDir(dir, d, s.Prefix, files); err != nil {
				return nil, err
			}
		}
	}
	for _, m := range site.Mounts {
		for f, body := range files {
			if f == m.Src {
				files[m.Dst] = body
			} else if rest, ok := strings.CutPrefix(f, m.Src+"/"); ok {
				files[m.Dst+"/"+rest] = body
			}
		}
	}
	if site.Prepare != nil {
		site.Prepare(files)
	}
	return files, nil
}

func clone(s Source, dir string) error {
	args := []string{"clone", "-q", "--depth", "1", "--filter=blob:none", "--sparse"}
	if s.Ref != "" {
		args = append(args, "--branch", s.Ref)
	}
	args = append(args, s.Repo, dir)
	if err := git("", args...); err != nil {
		return err
	}
	return git(dir, append([]string{"sparse-checkout", "set"}, s.Dirs...)...)
}

func git(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func readDir(root, dir, prefix string, files map[string]string) error {
	return filepath.WalkDir(filepath.Join(root, dir), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if info, err := d.Info(); err != nil || info.Size() > 2<<20 {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil || bytes.IndexByte(b, 0) >= 0 {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		files[prefix+filepath.ToSlash(rel)] = string(b)
		return nil
	})
}

// mapPages assigns a page path to every Markdown file under a root.
func mapPages(site Site, files map[string]string) map[string]string {
	out := map[string]string{}
	for f := range files {
		ext := path.Ext(f)
		if ext != ".md" && ext != ".mdx" {
			continue
		}
		for _, r := range site.Roots {
			dir := strings.TrimSuffix(r.Dir, "/") + "/"
			if !strings.HasPrefix(f, dir) {
				continue
			}
			rel := strings.TrimSuffix(strings.TrimPrefix(f, dir), ext)
			switch base := path.Base(rel); strings.ToLower(base) {
			case "_index", "index", "readme":
				rel = path.Dir(rel)
			}
			p := strings.Trim(path.Join(r.Page, rel), "/.")
			if p == "" {
				p = "index"
			}
			if !skipped(site, p) {
				out[f] = p
			}
			break
		}
	}
	return out
}

func skipped(site Site, p string) bool {
	for _, s := range site.Skip {
		if p == s || strings.HasPrefix(p, s+"/") {
			return true
		}
	}
	return false
}

var (
	fmTitleRe  = regexp.MustCompile(`(?m)^(title|sidebar_label|linkTitle)\s*[:=]\s*(.*?)\s*$`)
	h1Re       = regexp.MustCompile(`(?s)<h1[^>]*>(.*?)</h1>`)
	headingRe  = regexp.MustCompile(`(?s)<h([23]) id="([^"]+)">(.*?)</h[23]>`)
	tagRe      = regexp.MustCompile(`<[^>]+>`)
	parenTitle = regexp.MustCompile(`\s*\(.*\)$`)
)

// splitFrontMatter removes YAML (---) or TOML (+++) front matter and
// returns its title.
func splitFrontMatter(md string) (title, body string) {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	for _, fence := range []string{"---", "+++"} {
		if !strings.HasPrefix(md, fence+"\n") {
			continue
		}
		fm, rest, ok := strings.Cut(md[4:], "\n"+fence+"\n")
		if !ok {
			return "", md
		}
		got := map[string]string{}
		for _, m := range fmTitleRe.FindAllStringSubmatch(fm, -1) {
			if _, dup := got[m[1]]; !dup {
				got[m[1]] = strings.Trim(m[2], `"'`)
			}
		}
		for _, k := range []string{"title", "linkTitle", "sidebar_label"} {
			if got[k] != "" {
				return got[k], rest
			}
		}
		return "", rest
	}
	return "", md
}

func mdToHTML(md string) string {
	ext := (parser.CommonExtensions | parser.AutoHeadingIDs) &^ parser.MathJax
	p := parser.NewWithExtensions(ext)
	r := mdhtml.NewRenderer(mdhtml.RendererOptions{Flags: mdhtml.CommonFlags &^ mdhtml.UseXHTML})
	return string(markdown.Render(p.Parse([]byte(md)), r))
}

type heading struct{ text, id string }

func extractHeadings(body string) []heading {
	var hs []heading
	for _, m := range headingRe.FindAllStringSubmatch(body, -1) {
		if text := plain(m[3]); text != "" {
			hs = append(hs, heading{text, m[2]})
		}
	}
	return hs
}

func plain(html string) string {
	return strings.Join(strings.Fields(htmlUnescape(tagRe.ReplaceAllString(html, ""))), " ")
}

func htmlUnescape(s string) string {
	return strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(s)
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func humanize(s string) string {
	s = strings.NewReplacer("-", " ", "_", " ").Replace(s)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// sectionFor names the entry type after the first SectionDepth path
// segments, using that page's title when it exists.
func sectionFor(site Site, p string, titles map[string]string) string {
	if site.Section != nil {
		return site.Section(p)
	}
	depth := site.SectionDepth
	if depth == 0 {
		depth = 1
	}
	parts := strings.Split(p, "/")
	if len(parts) == 1 && !hasChildren(p, titles) {
		return "General"
	}
	if len(parts) > depth {
		parts = parts[:depth]
	}
	for len(parts) > 1 && !hasChildren(strings.Join(parts, "/"), titles) {
		parts = parts[:len(parts)-1]
	}
	var names []string
	for i := range parts {
		key := strings.Join(parts[:i+1], "/")
		t := titles[key]
		if t == "" || t == "Overview" {
			t = humanize(parts[i])
		}
		names = append(names, t)
	}
	return strings.Join(names, ": ")
}

func hasChildren(p string, titles map[string]string) bool {
	for q := range titles {
		if strings.HasPrefix(q, p+"/") {
			return true
		}
	}
	return false
}
