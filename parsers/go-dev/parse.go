package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

// httpGet fetches url and returns the body as a string. Any non-200
// is an error. Used for both go.dev and /ref/spec.
func httpGet(url string) (string, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "marvindocs-go-dev-parser")
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

// fetchArticle pulls the URL, parses the HTML, and returns just the
// inner contents of the first <article> tag. go.dev wraps every
// content page in an <article>, so this drops the site chrome
// (header, footer, sidebar) cleanly.
//
// If no <article> is found, the full body is returned — better than
// nothing for pages that use a different layout.
func fetchArticle(url string) (string, []extractedHeading, error) {
	raw, err := httpGet(url)
	if err != nil {
		return "", nil, err
	}
	body, headings, err := extractArticle(raw)
	if err != nil {
		return "", nil, err
	}
	return body, headings, nil
}

// extractedHeading is one h2/h3 we found inside the article. id is
// the html "id" attribute (becomes the URL fragment); text is the
// human-readable name.
type extractedHeading struct {
	level int
	id    string
	text  string
}

// extractArticle parses raw HTML and returns the rendered HTML of
// the first <article> element along with all h2/h3 ids+text inside
// it. We deliberately do NOT strip child tags or rewrite links —
// that's marvindocs' renderer job. The returned fragment is a plain
// HTML body fragment, suitable for db.json.
func extractArticle(raw string) (string, []extractedHeading, error) {
	root, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return "", nil, fmt.Errorf("parse html: %w", err)
	}
	article := findElement(root, "article")
	if article == nil {
		// fallback: pick <main> if present, else whole <body>.
		article = findElement(root, "main")
	}
	if article == nil {
		article = findElement(root, "body")
	}
	if article == nil {
		return "", nil, fmt.Errorf("no <article>/<main>/<body> in document")
	}

	var buf strings.Builder
	for c := article.FirstChild; c != nil; c = c.NextSibling {
		_ = html.Render(&buf, c)
	}
	return buf.String(), collectHeadings(article), nil
}

// findElement walks the DOM depth-first and returns the first
// element with the given tag name (or nil).
func findElement(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if got := findElement(c, tag); got != nil {
			return got
		}
	}
	return nil
}

// collectHeadings walks the subtree and returns one entry for every
// h2/h3 with a non-empty id attribute.
func collectHeadings(root *html.Node) []extractedHeading {
	var out []extractedHeading
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "h2" || n.Data == "h3") {
			id := attr(n, "id")
			if id != "" {
				level := 2
				if n.Data == "h3" {
					level = 3
				}
				out = append(out, extractedHeading{
					level: level,
					id:    id,
					text:  textContent(n),
				})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

// attr returns the value of name on n, or "" if absent.
func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// textContent concatenates all descendant text nodes, trimming and
// collapsing whitespace.
func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// stderr is the package-level alias used by run(); main.go never
// sees it directly.
var stderr io.Writer = os.Stderr

// addPage downloads a single go.dev page and appends it to the
// writer with one top-level entry plus one entry per heading.
// pageType is the section grouping shown in marvindocs search.
func addPage(w *devdocsfmt.Writer, baseURL, urlPath, slug, pageTitle, pageType string) error {
	body, headings, err := fetchArticle(baseURL + urlPath)
	if err != nil {
		return err
	}
	w.AddPage(slug, body)
	w.AddEntry(pageTitle, slug, pageType)
	for _, h := range headings {
		// Skip headings whose text is empty after normalization.
		if h.text == "" {
			continue
		}
		w.AddEntry(h.text, slug+"#"+h.id, pageType)
	}
	fmt.Fprintf(stderr, "  + %s (%d headings)\n", urlPath, len(headings))
	return nil
}

// nowUnix returns the current epoch second; isolated for test fakes.
var nowUnix = func() int64 { return time.Now().Unix() }
