package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
	"golang.org/x/net/html"
)

var client = &http.Client{Timeout: 45 * time.Second}

func fetch(address string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "marvindocs-metasploit-parser")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", address, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// The published sitemap still uses the original GitHub Pages origin.
func localPath(u, base *url.URL) (string, bool) {
	p := u.Path
	if u.Host == "rapid7.github.io" && strings.HasPrefix(p, "/metasploit-framework/") {
		p = strings.TrimPrefix(p, "/metasploit-framework/")
	} else if u.Host == base.Host && u.Scheme == base.Scheme && strings.HasPrefix(p, base.Path) {
		p = strings.TrimPrefix(p, base.Path)
	} else {
		return "", false
	}
	if p == "" || p == "index.html" {
		return "index", true
	}
	if !strings.HasPrefix(p, "docs/") || !strings.HasSuffix(p, ".html") {
		return "", false
	}
	return p, true
}

func run(f devdocsfmt.Flags) error {
	base, err := url.Parse(strings.TrimRight(f.Source, "/") + "/")
	if err != nil {
		return err
	}
	if base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return fmt.Errorf("--source must be an HTTP(S) site URL")
	}
	data, err := fetch(base.ResolveReference(&url.URL{Path: "sitemap.xml"}).String())
	if err != nil {
		return err
	}
	var sitemap struct {
		XMLName xml.Name `xml:"urlset"`
		URLs    []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal(data, &sitemap); err != nil {
		return fmt.Errorf("sitemap: %w", err)
	}
	pages := map[string]bool{"index": true}
	for _, item := range sitemap.URLs {
		u, err := url.Parse(strings.TrimSpace(item.Loc))
		if err != nil {
			return err
		}
		if p, ok := localPath(u, base); ok {
			pages[p] = true
		}
	}
	if len(pages) < 2 {
		return fmt.Errorf("sitemap contains no documentation pages")
	}
	paths := make([]string, 0, len(pages))
	for p := range pages {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	w := devdocsfmt.NewWriter(devdocsfmt.Doc{Name: "Metasploit Framework", Slug: "metasploit", Type: "metasploit", Version: f.Version, Release: f.Release,
		Links:       map[string]string{"home": "https://docs.metasploit.com/", "code": "https://github.com/rapid7/metasploit-framework"},
		Attribution: `Documentation from <a href="https://docs.metasploit.com/">Metasploit Framework</a>, Rapid7 and contributors.`,
	})
	for i, p := range paths {
		ref := p
		if p == "index" {
			ref = ""
		}
		u := base.ResolveReference(&url.URL{Path: ref})
		data, err := fetch(u.String())
		if err != nil {
			return err
		}
		body, title, headings, err := parsePage(data, u, base, p, pages)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		section := "Overview"
		parts := strings.Split(p, "/")
		if len(parts) > 2 {
			section = parts[1]
		}
		w.AddPage(p, body)
		w.AddEntry(title, p, section)
		for _, h := range headings {
			w.AddEntry(h.title, p+"#"+h.id, section)
		}
		fmt.Fprintf(os.Stderr, "[%d/%d] %s\n", i+1, len(paths), p)
	}
	// Do not replace an existing corpus if any upstream page fails.
	return w.Flush(f.Out, f.Force)
}

type heading struct{ title, id string }

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

func textContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(textContent(c))
	}
	return b.String()
}

func parsePage(data []byte, pageURL, base *url.URL, pagePath string, pages map[string]bool) (string, string, []heading, error) {
	doc, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return "", "", nil, err
	}
	var main *html.Node
	title := ""
	var find func(*html.Node)
	find = func(n *html.Node) {
		if attr(n, "id") == "main-content" {
			main = n
		}
		if n.Type == html.ElementNode && n.Data == "title" && n.Parent != nil && n.Parent.Data == "head" {
			title = strings.TrimSpace(strings.SplitN(textContent(n), "|", 2)[0])
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(doc)
	if main == nil || title == "" {
		return "", "", nil, fmt.Errorf("missing main-content or page title")
	}
	var headings []heading
	var clean func(*html.Node)
	clean = func(n *html.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			if c.Type == html.ElementNode && (c.Data == "footer" || c.Data == "script" || c.Data == "style" || c.Data == "svg" || strings.Contains(" "+attr(c, "class")+" ", " anchor-heading ")) {
				n.RemoveChild(c)
			} else {
				clean(c)
			}
			c = next
		}
		if n.Type != html.ElementNode {
			return
		}
		for i, a := range n.Attr {
			if a.Key != "href" && a.Key != "src" {
				continue
			}
			if strings.HasPrefix(a.Val, "#") {
				continue
			}
			u, err := pageURL.Parse(a.Val)
			if err != nil {
				continue
			}
			if p, ok := localPath(u, base); a.Key == "href" && ok && pages[p] && u.RawQuery == "" {
				rel, err := filepath.Rel(path.Dir(pagePath), p)
				if err == nil {
					u = &url.URL{Path: filepath.ToSlash(rel), Fragment: u.Fragment}
				}
			}
			n.Attr[i].Val = u.String()
		}
		if len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6' {
			if id := attr(n, "id"); id != "" {
				headings = append(headings, heading{strings.Join(strings.Fields(textContent(n)), " "), id})
			}
		}
	}
	clean(main)
	var body strings.Builder
	for c := main.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&body, c); err != nil {
			return "", "", nil, err
		}
	}
	if strings.TrimSpace(textContent(main)) == "" {
		return "", "", nil, fmt.Errorf("empty page content")
	}
	return body.String(), title, headings, nil
}
