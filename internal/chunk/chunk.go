// Package chunk splits Markdown pages into heading-delimited sections and
// scores them against a query, so that the MCP get_docs tool can return a
// handful of relevant fragments that fit a token budget instead of whole
// pages.
package chunk

import (
	"strings"
	"unicode/utf8"
)

// Section is one heading-delimited slice of a Markdown page.
type Section struct {
	// Heading is the heading text without the leading '#'s; empty for
	// the text before the first heading.
	Heading string
	// Text is the section body including its heading line.
	Text string
}

// maxSectionChars bounds a single section (~2000 tokens). Longer
// sections are split further at blank lines so that one heading-less
// page cannot eat the whole budget with irrelevant text.
const maxSectionChars = 8000

// Split cuts md into sections at ATX headings (lines starting with
// '#'). Headings inside fenced code blocks are ignored. Sections
// longer than maxSectionChars are split again at paragraph breaks;
// the pieces share the heading.
func Split(md string) []Section {
	var out []Section
	var cur strings.Builder
	var heading string
	inFence := false

	flush := func() {
		text := strings.TrimSpace(cur.String())
		cur.Reset()
		if text == "" {
			return
		}
		for _, piece := range splitLong(text) {
			out = append(out, Section{Heading: heading, Text: piece})
		}
	}

	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
		}
		if !inFence && isHeading(trimmed) {
			flush()
			heading = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
	}
	flush()
	return out
}

// isHeading reports whether a trimmed line is an ATX heading: 1–6 '#'
// followed by a space.
func isHeading(line string) bool {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	return n >= 1 && n <= 6 && n < len(line) && line[n] == ' '
}

// splitLong breaks text at blank lines into pieces of at most
// maxSectionChars. A single paragraph longer than the cap is kept
// whole: cutting mid-sentence helps nobody.
func splitLong(text string) []string {
	if len(text) <= maxSectionChars {
		return []string{text}
	}
	var pieces []string
	var cur strings.Builder
	for _, para := range strings.Split(text, "\n\n") {
		if cur.Len() > 0 && cur.Len()+len(para)+2 > maxSectionChars {
			pieces = append(pieces, strings.TrimSpace(cur.String()))
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteString("\n\n")
		}
		cur.WriteString(para)
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		pieces = append(pieces, s)
	}
	return pieces
}

// Terms lowercases and splits a query into words for Score.
func Terms(query string) []string {
	return strings.Fields(strings.ToLower(query))
}

// Score counts how many query terms occur in the section as a word
// prefix (the same notion of a match the FTS index uses). Terms found
// in the heading count twice. Zero means the section is unrelated.
func Score(s Section, terms []string) int {
	if len(terms) == 0 {
		return 0
	}
	score := 0
	body := strings.ToLower(s.Text)
	head := strings.ToLower(s.Heading)
	for _, t := range terms {
		if containsWordPrefix(body, t) {
			score++
		}
		if containsWordPrefix(head, t) {
			score++
		}
	}
	return score
}

// containsWordPrefix reports whether some word in text starts with
// term. Words are delimited by whitespace and common punctuation.
func containsWordPrefix(text, term string) bool {
	for _, w := range strings.FieldsFunc(text, isSeparator) {
		if strings.HasPrefix(w, term) {
			return true
		}
	}
	return false
}

func isSeparator(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '(', ')', '[', ']', '{', '}', '<', '>', '"', '\'', '`', ',', ';', ':', '*', '|':
		return true
	}
	return false
}

// Tokens estimates the LLM token count of s as characters/4. Rough,
// but good enough to keep responses near the requested budget.
func Tokens(s string) int {
	return (utf8.RuneCountInString(s) + 3) / 4
}

// Truncate cuts s so that it fits in tokens, ending at a word boundary
// and marking the cut with "…". Returns s unchanged when it already
// fits.
func Truncate(s string, tokens int) string {
	if Tokens(s) <= tokens {
		return s
	}
	limit := tokens * 4
	n := 0
	cut := 0
	for i, r := range s {
		if n >= limit {
			break
		}
		if r == ' ' || r == '\n' {
			cut = i
		}
		n++
	}
	if cut == 0 {
		return ""
	}
	return strings.TrimSpace(s[:cut]) + " …"
}
