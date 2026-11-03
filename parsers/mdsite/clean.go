package main

import (
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	commentRe   = regexp.MustCompile(`(?s)<!--.*?-->`)
	includeRe   = regexp.MustCompile(`\{\{[<%]\s*(?:include|readfile)\s+(?:file=)?"([^"]+)"[^}]*[%>]\}\}|--8<--\s+"([^"]+)"|\{%\s*include-markdown\s+"([^"]+)"[^%]*%\}|\{!([^!\s]+)!\}|(?m)^[ \t]*(?:[-*][ \t]+)?\x60?@include\s+['"]([^'"]+)['"]\x60?`)
	textArgRe   = regexp.MustCompile(`\{\{[<%]\s*[\w-]+\s+[^}]*?\btext="([^"]*)"[^}]*[%>]\}\}`)
	tabRe       = regexp.MustCompile(`\{\{[<%]\s*tab\s+name="([^"]*)"[^}]*[%>]\}\}`)
	shortcodeRe = regexp.MustCompile(`(?s)\{\{[<%].*?[%>]\}\}`)
	liquidRe    = regexp.MustCompile(`\{%-?.*?-?%\}`)
	mdxLineRe   = regexp.MustCompile(`^(import|export)\s`)
	jsxTagRe    = regexp.MustCompile(`</?[A-Z][A-Za-z.]*(\s[^<>]*)?/?>`)
	admonRe     = regexp.MustCompile(`^(\s*)(?:!!!|\?\?\?\+?|===)\s*([\w-]*)\s*(?:"([^"]*)")?\s*$`)
	colonRe     = regexp.MustCompile(`^(\s*):::+\s*([\w-]*)\s*(.*)$`)
	alertRe     = regexp.MustCompile(`^((?:\s*>)+\s*)\[!(\w+)\]`)
	fenceRe     = regexp.MustCompile("^[\\s>]*(```|~~~)")
)

// cleanMarkdown turns site-generator Markdown into plain CommonMark:
// resolves includes, drops shortcodes, MDX imports and JSX wrappers,
// and rewrites admonitions and tabs as bold labels.
func cleanMarkdown(site Site, md string, files map[string]string, depth int) string {
	if site.Clean != nil {
		md = site.Clean(md, files)
	}
	md = commentRe.ReplaceAllString(md, "")
	md = includeRe.ReplaceAllStringFunc(md, func(s string) string {
		m := includeRe.FindStringSubmatch(s)
		name := m[1] + m[2] + m[3] + m[4] + m[5]
		if depth > 2 {
			return ""
		}
		for _, d := range append(site.IncludeDirs[:len(site.IncludeDirs):len(site.IncludeDirs)], "") {
			if body, ok := files[path.Join(d, name)]; ok {
				if ext := path.Ext(name); ext != ".md" && ext != ".mdx" {
					return strings.TrimRight(body, "\n")
				}
				_, body = splitFrontMatter(body)
				return cleanMarkdown(site, body, files, depth+1)
			}
		}
		return ""
	})
	md = textArgRe.ReplaceAllString(md, "$1")
	md = tabRe.ReplaceAllString(md, "\n**$1**\n")
	md = shortcodeRe.ReplaceAllString(md, "")
	if site.Liquid {
		md = liquidRe.ReplaceAllString(md, "")
	}

	lines := strings.Split(md, "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	dedent := -1 // indentation of the admonition marker being unwrapped
	for _, l := range lines {
		if dedent >= 0 {
			ind := len(l) - len(strings.TrimLeft(l, " \t"))
			switch {
			case strings.TrimSpace(l) == "":
			case ind >= dedent+4:
				l = l[:dedent] + l[dedent+4:]
			default:
				dedent = -1
			}
		}
		if fenceRe.MatchString(l) {
			inFence = !inFence
		}
		if inFence {
			out = append(out, l)
			continue
		}
		if mdxLineRe.MatchString(l) {
			continue
		}
		if m := admonRe.FindStringSubmatch(l); m != nil {
			dedent = len(m[1])
			out = append(out, m[1]+label(m[2], m[3]), "")
			continue
		}
		if m := colonRe.FindStringSubmatch(l); m != nil {
			if m[2] != "" {
				out = append(out, m[1]+label(m[2], m[3]), "")
			}
			continue
		}
		l = alertRe.ReplaceAllString(l, "$1**$2:**")
		l = jsxTagRe.ReplaceAllString(l, "")
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func label(kind, title string) string {
	title = strings.Trim(strings.TrimSpace(title), `"[]`)
	if title == "" {
		title = humanize(kind)
	}
	if title == "" {
		return ""
	}
	return "**" + title + "**"
}

var hrefRe = regexp.MustCompile(`(href|src)="([^"]*)"`)

// rewriteLinks makes links to pages of this doc relative and every
// other site link absolute. Links with a .md/.mdx extension are
// relative to the source file, others to the page URL (Hugo style).
func rewriteLinks(site Site, body, file, page string, fileToPage map[string]string, known map[string]bool) string {
	return hrefRe.ReplaceAllStringFunc(body, func(s string) string {
		m := hrefRe.FindStringSubmatch(s)
		attr, u := m[1], m[2]
		if self := site.BaseURL + site.URLPrefix; strings.HasPrefix(u, self) {
			u = strings.TrimPrefix(u, site.BaseURL)
		}
		if u == "" || strings.HasPrefix(u, "#") || strings.Contains(u, "://") || strings.HasPrefix(u, "//") || strings.HasPrefix(u, "mailto:") {
			return s
		}
		target, frag, _ := strings.Cut(u, "#")
		target, _, _ = strings.Cut(target, "?")
		var p string
		if ext := path.Ext(target); ext == ".md" || ext == ".mdx" {
			if strings.HasPrefix(target, "/") {
				// Content-root relative: try every root and its parent.
				for _, r := range site.Roots {
					for _, d := range []string{r.Dir, path.Dir(r.Dir)} {
						if q, ok := fileToPage[path.Join(d, target)]; ok && p == "" {
							p = q
						}
					}
				}
			} else {
				p = fileToPage[path.Join(path.Dir(file), target)]
			}
			if p == "" {
				return attr + `="` + site.Home + `"`
			}
		} else {
			urlPath := target
			if !strings.HasPrefix(target, "/") {
				urlPath = path.Join(site.URLPrefix, pageURL(page), target)
			}
			p = strings.Trim(strings.TrimPrefix(path.Clean(urlPath), strings.TrimSuffix(site.URLPrefix, "/")), "/")
			if p == "" {
				p = "index"
			}
			if !known[p] {
				p = ""
			}
			if p == "" || attr == "src" {
				abs := urlPath
				if !strings.HasPrefix(abs, "/") {
					abs = "/" + abs
				}
				if frag != "" {
					abs += "#" + frag
				}
				return attr + `="` + site.BaseURL + path.Clean(abs) + `"`
			}
		}
		if p == "" {
			return s
		}
		rel, err := filepath.Rel(path.Dir(page), p)
		if err != nil {
			return s
		}
		u = filepath.ToSlash(rel)
		if frag != "" {
			u += "#" + frag
		}
		return attr + `="` + u + `"`
	})
}

func pageURL(p string) string {
	if p == "index" {
		return ""
	}
	return p
}
