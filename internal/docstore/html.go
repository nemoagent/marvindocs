package docstore

import (
	"regexp"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"golang.org/x/net/html"
)

// dataURI matches inline base64 payloads (data:image/png;base64,...).
var dataURI = regexp.MustCompile(`data:[\w.+-]+/[\w.+-]+;base64,[A-Za-z0-9+/=]+`)

// StripDataURIs blanks inline base64 payloads. Some pages embed hundreds
// of images this way (matplotlib: 6 MB of HTML for 27 KB of text); the
// bytes are useless to an LLM and would dominate the token budget.
func StripDataURIs(fragment string) string {
	return dataURI.ReplaceAllLiteralString(fragment, "data:,")
}

// ToMarkdown converts a devdocs HTML fragment to Markdown.
// Used by the MCP get_page tool and the web UI when ?format=md is requested.
func ToMarkdown(fragment string) (string, error) {
	return htmltomarkdown.ConvertString(StripDataURIs(fragment))
}

// ToPlainText strips all HTML tags and returns a whitespace-normalized
// plain-text version of the fragment. Used when building the full-text index.
func ToPlainText(fragment string) string {
	z := html.NewTokenizer(strings.NewReader(fragment))
	var b strings.Builder
	b.Grow(len(fragment))

	skip := 0 // depth inside <script>/<style>
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return normalizeWhitespace(b.String())
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "script", "style":
				if tt == html.StartTagToken {
					skip++
				}
			}
			// add a space so that inline tags don't glue words together
			if skip == 0 {
				b.WriteByte(' ')
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if string(name) == "script" || string(name) == "style" {
				if skip > 0 {
					skip--
				}
			}
			if skip == 0 {
				b.WriteByte(' ')
			}
		case html.TextToken:
			if skip == 0 {
				b.Write(z.Text())
			}
		}
	}
}

// normalizeWhitespace collapses runs of whitespace into single spaces and
// trims the result.
func normalizeWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := true // suppress leading whitespace
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\u00a0' {
			if !space {
				b.WriteByte(' ')
				space = true
			}
			continue
		}
		b.WriteRune(r)
		space = false
	}
	return strings.TrimRight(b.String(), " ")
}
