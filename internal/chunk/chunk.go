// Package chunk cuts Markdown pages into small offset-addressed chunks for
// the full-text index, and trims fragments to a token budget so that the
// MCP get_docs tool can return a handful of relevant pieces instead of
// whole pages.
package chunk

import (
	"strings"
	"unicode/utf8"
)

// Chunk is a slice of a Markdown page addressed by byte offsets, so the
// index can store positions only and re-slice the page at query time.
type Chunk struct {
	// Heading is the nearest ATX heading text (without '#'); empty
	// before the first heading.
	Heading  string
	Off, Len int
}

// maxChunkChars is the target chunk size in bytes; chunks shorter than
// minChunkChars (heading-only sections, one-line notes) are merged into
// a neighbour, since bm25 overrates tiny texts and they are useless on
// their own.
const (
	maxChunkChars = 1200
	minChunkChars = 200
)

// Chunks cuts md at ATX headings (ignoring headings inside ``` / ~~~
// fences) and then at blank lines, packing paragraphs into chunks of at
// most maxChunkChars. A single paragraph is kept whole up to
// 2*maxChunkChars; a longer one (typically a table) is hard-cut at
// maxChunkChars. Offsets index into md; chunk text is md[Off:Off+Len],
// without leading/trailing whitespace. Empty chunks are dropped.
func Chunks(md string) []Chunk {
	var out []Chunk
	var heading string // heading of the section being collected
	start := 0
	inFence := false

	for off := 0; off < len(md); {
		end := len(md)
		if n := strings.IndexByte(md[off:], '\n'); n >= 0 {
			end = off + n + 1
		}
		trimmed := strings.TrimSpace(md[off:end])
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
		}
		if !inFence && isHeading(trimmed) {
			out = appendSection(out, md, start, off, heading)
			start = off
			heading = headingText(trimmed)
		}
		off = end
	}
	return mergeShort(appendSection(out, md, start, len(md), heading))
}

// mergeShort joins chunks shorter than minChunkChars with the following
// chunk (the last one with the preceding chunk). The merged chunk takes
// the heading of the later part, which the body belongs to.
func mergeShort(in []Chunk) []Chunk {
	var out []Chunk
	for k := 0; k < len(in); k++ {
		c := in[k]
		for c.Len < minChunkChars && k+1 < len(in) {
			k++
			c = Chunk{Heading: in[k].Heading, Off: c.Off, Len: in[k].Off + in[k].Len - c.Off}
		}
		if c.Len < minChunkChars && len(out) > 0 {
			prev := &out[len(out)-1]
			prev.Len = c.Off + c.Len - prev.Off
			continue
		}
		out = append(out, c)
	}
	return out
}

// appendSection packs the paragraphs of md[start:end] into chunks.
func appendSection(out []Chunk, md string, start, end int, heading string) []Chunk {
	emit := func(s, e int) {
		for s < e && isSpace(md[s]) {
			s++
		}
		for e > s && isSpace(md[e-1]) {
			e--
		}
		if s < e {
			out = append(out, Chunk{Heading: heading, Off: s, Len: e - s})
		}
	}

	cs, ce := -1, -1 // current chunk, cs < 0 when empty
	para := func(ps, pe int) {
		if cs >= 0 && pe-cs > maxChunkChars {
			emit(cs, ce)
			cs = -1
		}
		if cs < 0 {
			cs = ps
		}
		ce = pe
		for ce-cs > 2*maxChunkChars {
			cut := cs + maxChunkChars
			for cut > cs && !utf8.RuneStart(md[cut]) {
				cut--
			}
			emit(cs, cut)
			cs = cut
		}
	}

	ps := -1 // start of the paragraph being read, -1 between paragraphs
	pe := 0
	for off := start; off < end; {
		le := end
		if n := strings.IndexByte(md[off:end], '\n'); n >= 0 {
			le = off + n + 1
		}
		if strings.TrimSpace(md[off:le]) == "" {
			if ps >= 0 {
				para(ps, pe)
				ps = -1
			}
		} else {
			if ps < 0 {
				ps = off
			}
			pe = le
		}
		off = le
	}
	if ps >= 0 {
		para(ps, pe)
	}
	if cs >= 0 {
		emit(cs, ce)
	}
	return out
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// headingText returns the text of a trimmed ATX heading line.
func headingText(line string) string {
	return strings.TrimSpace(strings.TrimLeft(line, "#"))
}

// Title returns the first ATX heading of md ("" if none).
func Title(md string) string {
	inFence := false
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
		}
		if !inFence && isHeading(trimmed) {
			return headingText(trimmed)
		}
	}
	return ""
}

// HeadingAt returns the nearest ATX heading line at or before off ("" if
// none). Unlike Chunks it does not track code fences.
func HeadingAt(md string, off int) string {
	off = min(max(off, 0), len(md))
	if n := strings.IndexByte(md[off:], '\n'); n >= 0 {
		off += n // include the line starting at off
	} else {
		off = len(md)
	}
	heading := ""
	for _, line := range strings.Split(md[:off], "\n") {
		if trimmed := strings.TrimSpace(line); isHeading(trimmed) {
			heading = headingText(trimmed)
		}
	}
	return heading
}

// isHeading reports whether a trimmed line is an ATX heading: 1-6 '#'
// followed by a space.
func isHeading(line string) bool {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	return n >= 1 && n <= 6 && n < len(line) && line[n] == ' '
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
