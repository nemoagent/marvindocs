package main

import (
	"fmt"
	"regexp"
	"strings"
)

// heading is one heading we plan to expose as a search entry plus an
// anchor on the rendered page.
type heading struct {
	text  string
	id    string
	level int
}

// rstToHTML renders a minimal subset of reStructuredText (with Snooty
// extensions used by mongodb/docs) into an HTML fragment. It is not a
// full reST implementation; the goal is to produce readable,
// searchable docs without pulling docutils into the build.
//
// Returns the page title (first H1, if any), HTML body, and a list
// of headings with anchors for search entry registration.
func rstToHTML(src string) (string, string, []heading) {
	src = stripFieldList(src)
	src = stripComments(src)
	src = stripDirectives(src)
	src = expandSubstitutions(src)

	lines := strings.Split(src, "\n")

	var (
		out      strings.Builder
		title    string
		headings []heading
		anchors  = map[string]int{}
		para     []string
	)

	flushPara := func() {
		if len(para) == 0 {
			return
		}
		text := strings.TrimSpace(strings.Join(para, " "))
		para = para[:0]
		if text == "" {
			return
		}
		fmt.Fprintf(&out, "<p>%s</p>\n", renderInline(text))
	}

	// Track pending target like ".. _foo:" so the next heading uses it
	// as its anchor id.
	pendingTarget := ""

	listMode := "" // "" | "ul" | "ol"
	listClose := func() {
		if listMode != "" {
			fmt.Fprintf(&out, "</%s>\n", listMode)
			listMode = ""
		}
	}

	i := 0
	for i < len(lines) {
		line := lines[i]
		trim := strings.TrimSpace(line)

		// Target reference: ".. _slug:"
		if m := targetRe.FindStringSubmatch(trim); m != nil {
			flushPara()
			listClose()
			pendingTarget = m[1]
			i++
			continue
		}

		// Code block start: literal "::" at end of paragraph or
		// ".. code-block:: lang"
		if m := codeBlockRe.FindStringSubmatch(trim); m != nil {
			flushPara()
			listClose()
			lang := strings.TrimSpace(m[1])
			i++
			// optional :options:
			for i < len(lines) && optionRe.MatchString(lines[i]) {
				i++
			}
			// blank line then indented block
			for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
				i++
			}
			block, n := readIndentedBlock(lines[i:])
			i += n
			if lang == "" {
				fmt.Fprintf(&out, "<pre><code>%s</code></pre>\n", htmlEscape(block))
			} else {
				fmt.Fprintf(&out, "<pre><code class=\"language-%s\">%s</code></pre>\n", htmlEscape(lang), htmlEscape(block))
			}
			continue
		}

		// Heading detection: a non-blank line followed by an underline
		// of repeated punctuation of length >= len(text).
		if i+1 < len(lines) {
			next := lines[i+1]
			if isUnderline(next, trim) {
				flushPara()
				listClose()
				level := headingLevel(next[0])
				text := trim
				if level == 1 && i >= 1 && isUnderline(lines[i-1], trim) {
					// (already handled by overline above)
				}
				id := pendingTarget
				if id == "" {
					id = slugify(text, anchors)
				} else {
					anchors[id]++
					pendingTarget = ""
				}
				if level == 1 && title == "" {
					title = text
				}
				headings = append(headings, heading{text: text, id: id, level: level})
				fmt.Fprintf(&out, "<h%d id=%q>%s</h%d>\n", level, id, renderInline(text), level)
				i += 2
				continue
			}
		}

		// Overline + heading + underline (used for top-level titles)
		if isOverline(line) && i+2 < len(lines) {
			text := strings.TrimSpace(lines[i+1])
			under := lines[i+2]
			if text != "" && isUnderline(under, text) {
				flushPara()
				listClose()
				level := 1
				id := pendingTarget
				if id == "" {
					id = slugify(text, anchors)
				} else {
					anchors[id]++
					pendingTarget = ""
				}
				if title == "" {
					title = text
				}
				headings = append(headings, heading{text: text, id: id, level: level})
				fmt.Fprintf(&out, "<h%d id=%q>%s</h%d>\n", level, id, renderInline(text), level)
				i += 3
				continue
			}
		}

		// Bullet list
		if m := bulletRe.FindStringSubmatch(line); m != nil {
			flushPara()
			if listMode != "ul" {
				listClose()
				out.WriteString("<ul>\n")
				listMode = "ul"
			}
			item := m[2]
			i++
			// continuation lines that are indented further
			indent := len(m[1]) + 2
			for i < len(lines) {
				cont := lines[i]
				if strings.TrimSpace(cont) == "" {
					break
				}
				if leadingSpaces(cont) < indent {
					break
				}
				item += " " + strings.TrimSpace(cont)
				i++
			}
			fmt.Fprintf(&out, "  <li>%s</li>\n", renderInline(strings.TrimSpace(item)))
			continue
		}

		// Numeric list
		if m := numListRe.FindStringSubmatch(line); m != nil {
			flushPara()
			if listMode != "ol" {
				listClose()
				out.WriteString("<ol>\n")
				listMode = "ol"
			}
			item := m[2]
			i++
			indent := len(m[1]) + len(m[0]) - len(m[2])
			for i < len(lines) {
				cont := lines[i]
				if strings.TrimSpace(cont) == "" {
					break
				}
				if leadingSpaces(cont) < indent {
					break
				}
				item += " " + strings.TrimSpace(cont)
				i++
			}
			fmt.Fprintf(&out, "  <li>%s</li>\n", renderInline(strings.TrimSpace(item)))
			continue
		}

		// Blank line: paragraph break, but keep an open list open if
		// the very next non-blank line continues the same list type.
		if trim == "" {
			flushPara()
			if listMode != "" {
				j := i + 1
				for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
					j++
				}
				if j < len(lines) {
					if listMode == "ul" && bulletRe.MatchString(lines[j]) {
						i = j
						continue
					}
					if listMode == "ol" && numListRe.MatchString(lines[j]) {
						i = j
						continue
					}
				}
				listClose()
			}
			i++
			continue
		}

		// Default: regular paragraph text.
		para = append(para, trim)
		i++
	}
	flushPara()
	listClose()
	return title, out.String(), headings
}

// stripFieldList removes top-of-file ":field: value" lines (e.g.
// ":orphan:" or ":author: Foo") that reST uses for document metadata.
// Only removes leading metadata block, not field lists inside content.
func stripFieldList(s string) string {
	lines := strings.Split(s, "\n")
	i := 0
	for i < len(lines) {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			i++
			continue
		}
		if fieldRe.MatchString(t) {
			i++
			continue
		}
		break
	}
	return strings.Join(lines[i:], "\n")
}

var fieldRe = regexp.MustCompile(`^:[a-zA-Z][a-zA-Z0-9_-]*:(\s|$)`)

// stripComments removes ".. (line that is a comment, not a directive)".
// A comment is a ".." line whose continuation does not start with a
// known directive form (“name::“).
func stripComments(s string) string {
	// reST comments: line starting with ".." and a space followed by
	// arbitrary text, no "::" inside the first token. We just drop
	// such lines and any indented continuation block.
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		line := lines[i]
		trim := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trim, ".. ") || trim == ".." {
			// Is this actually a directive? Format: ".. name::"
			if directiveRe.MatchString(trim) {
				out = append(out, line)
				i++
				continue
			}
			// Is this a target? ".. _name:"
			if targetRe.MatchString(trim) {
				out = append(out, line)
				i++
				continue
			}
			// Is this a substitution? ".. |name| ..."
			if strings.HasPrefix(trim, ".. |") {
				out = append(out, line)
				i++
				continue
			}
			// Plain comment — eat this line and any indented continuation.
			indent := leadingSpaces(line)
			i++
			for i < len(lines) {
				if strings.TrimSpace(lines[i]) == "" {
					i++
					continue
				}
				if leadingSpaces(lines[i]) > indent {
					i++
					continue
				}
				break
			}
			continue
		}
		out = append(out, line)
		i++
	}
	return strings.Join(out, "\n")
}

// stripDirectives drops directives we cannot render usefully (toctree,
// facet, meta, contents, dismissible-skills-card, cta-banner, include,
// default-domain, etc). Some directives we keep as-is; those are
// handled in the main loop (code-block).
//
// For dropped directives we also consume the indented body so it
// doesn't leak into output.
func stripDirectives(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		line := lines[i]
		trim := strings.TrimLeft(line, " ")
		if m := directiveRe.FindStringSubmatch(trim); m != nil {
			name := m[1]
			if _, keep := keepDirective[name]; keep {
				out = append(out, line)
				i++
				continue
			}
			// Drop the directive and any indented continuation block.
			indent := leadingSpaces(line)
			i++
			for i < len(lines) {
				if strings.TrimSpace(lines[i]) == "" {
					// allow one blank line then keep eating if next is still indented
					j := i + 1
					for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
						j++
					}
					if j < len(lines) && leadingSpaces(lines[j]) > indent {
						i = j
						continue
					}
					break
				}
				if leadingSpaces(lines[i]) > indent {
					i++
					continue
				}
				break
			}
			continue
		}
		out = append(out, line)
		i++
	}
	return strings.Join(out, "\n")
}

// expandSubstitutions replaces |name| references with their literal
// substitution definition, when defined inline as
// ".. |name| replace:: text". Other substitutions are left as-is.
func expandSubstitutions(s string) string {
	subs := map[string]string{}
	subRe := regexp.MustCompile(`(?m)^\.\. \|([^|]+)\| replace:: (.+)$`)
	for _, m := range subRe.FindAllStringSubmatch(s, -1) {
		subs[m[1]] = m[2]
	}
	// Strip the definition lines.
	s = subRe.ReplaceAllString(s, "")
	if len(subs) == 0 {
		return s
	}
	for name, val := range subs {
		s = strings.ReplaceAll(s, "|"+name+"|", val)
	}
	return s
}

var keepDirective = map[string]bool{
	"code-block": true,
	"code":       true,
	"sourcecode": true,
}

var (
	directiveRe = regexp.MustCompile(`^\.\. ([a-zA-Z][a-zA-Z0-9_-]*)::`)
	codeBlockRe = regexp.MustCompile(`^\.\. (?:code-block|code|sourcecode)::\s*(.*)$`)
	optionRe    = regexp.MustCompile(`^\s+:[a-zA-Z-]+:`)
	targetRe    = regexp.MustCompile(`^\.\. _([a-zA-Z0-9_.\-+/]+):\s*$`)
	bulletRe    = regexp.MustCompile(`^(\s*)[-*+] (.*)$`)
	numListRe   = regexp.MustCompile(`^(\s*)[0-9]+\.\s+(.*)$`)
)

func isUnderline(line, text string) bool {
	if len(line) == 0 || len(text) == 0 {
		return false
	}
	c := line[0]
	if !isHeadingChar(c) {
		return false
	}
	for i := 0; i < len(line); i++ {
		if line[i] != c {
			return false
		}
	}
	return len(line) >= len(text)
}

func isOverline(line string) bool {
	if len(line) == 0 {
		return false
	}
	c := line[0]
	if !isHeadingChar(c) {
		return false
	}
	for i := 0; i < len(line); i++ {
		if line[i] != c {
			return false
		}
	}
	return len(line) >= 3
}

func isHeadingChar(c byte) bool {
	return strings.ContainsRune("=-~`^\"'+*#_:", rune(c))
}

func headingLevel(c byte) int {
	switch c {
	case '=':
		return 1
	case '-':
		return 2
	case '~':
		return 3
	case '^':
		return 4
	case '"':
		return 5
	default:
		return 6
	}
}

func leadingSpaces(s string) int {
	n := 0
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}

func readIndentedBlock(lines []string) (string, int) {
	if len(lines) == 0 {
		return "", 0
	}
	indent := -1
	var out []string
	i := 0
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			out = append(out, "")
			i++
			continue
		}
		spaces := leadingSpaces(line)
		if indent == -1 {
			if spaces == 0 {
				break
			}
			indent = spaces
		}
		if spaces < indent {
			break
		}
		out = append(out, line[indent:])
		i++
	}
	// Trim trailing blank lines.
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n"), i
}

func slugify(s string, used map[string]int) string {
	s = strings.ToLower(s)
	s = nonAlnumRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "section"
	}
	used[s]++
	if used[s] > 1 {
		s = fmt.Sprintf("%s-%d", s, used[s]-1)
	}
	return s
}

var nonAlnumRe = regexp.MustCompile(`[^a-z0-9]+`)

// renderInline handles inline reST: literal “code“, *emphasis*,
// **strong**, role refs `text <target>`_ and :role:`target`. The role
// target is stripped because we cannot resolve it across docs without
// the full Snooty index; we keep the displayed text.
//
// Tokens are replaced before html escaping so that markup characters
// inside reST roles (like the “<target>“ part) don't get escaped
// into “&lt;target&gt;“.
func renderInline(s string) string {
	type tok struct {
		key  string
		repl string
	}
	var toks []tok
	stash := func(html string) string {
		key := fmt.Sprintf("\x00T%d\x00", len(toks))
		toks = append(toks, tok{key: key, repl: html})
		return key
	}

	// `text <url>`_ — external link.
	s = externalLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := externalLinkRe.FindStringSubmatch(m)
		return stash(fmt.Sprintf(`<a href=%q>%s</a>`,
			htmlEscape(sub[2]), htmlEscape(sub[1])))
	})
	// :role:`text <target>` — keep visible text only.
	s = roleWithTargetRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := roleWithTargetRe.FindStringSubmatch(m)
		if isCodeRole(sub[1]) {
			return stash("<code>" + htmlEscape(sub[2]) + "</code>")
		}
		return stash(htmlEscape(sub[2]))
	})
	// :role:`text` — keep text; wrap in <code> for code-like roles.
	s = roleRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := roleRe.FindStringSubmatch(m)
		if isCodeRole(sub[1]) {
			return stash("<code>" + htmlEscape(sub[2]) + "</code>")
		}
		return stash(htmlEscape(sub[2]))
	})
	// ``literal``
	s = literalRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := literalRe.FindStringSubmatch(m)
		return stash("<code>" + htmlEscape(sub[1]) + "</code>")
	})
	// **strong**
	s = strongRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := strongRe.FindStringSubmatch(m)
		return stash("<strong>" + htmlEscape(sub[1]) + "</strong>")
	})
	// *emphasis*
	s = emphRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := emphRe.FindStringSubmatch(m)
		return stash("<em>" + htmlEscape(sub[1]) + "</em>")
	})

	s = htmlEscape(s)
	for _, t := range toks {
		s = strings.ReplaceAll(s, t.key, t.repl)
	}
	return s
}

var (
	roleWithTargetRe = regexp.MustCompile(":([a-zA-Z]+):`([^`<]+?)\\s+<[^`>]+>`")
	roleRe           = regexp.MustCompile(":([a-zA-Z]+):`([^`]+)`")
	externalLinkRe   = regexp.MustCompile("`([^`<]+?)\\s+<([^`>]+)>`_")
	literalRe        = regexp.MustCompile("``([^`]+)``")
	strongRe         = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	emphRe           = regexp.MustCompile(`\*([^*\s][^*]*[^*\s]|[^*\s])\*`)
)

// isCodeRole returns true for Snooty/MongoDB roles whose payload is
// almost always a code-like identifier (operator name, command name,
// configuration key, ...). Such payloads are wrapped in <code> for
// readability. Plain prose roles like :ref:, :doc:, :guides: are left
// as-is to avoid turning narrative text into monospace.
func isCodeRole(name string) bool {
	switch name {
	case "binary", "command", "compass-icon", "data", "dbcommand",
		"expression", "file", "limit", "method", "operator", "option",
		"parameter", "phpmethod", "phpclass", "program", "pipeline",
		"query", "readconcern", "writeconcern", "setting", "type",
		"variable", "node", "perl", "node-api", "ruby", "scala",
		"csharp", "go-api":
		return true
	}
	return false
}
