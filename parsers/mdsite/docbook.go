package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
)

// A small DocBook (refentry) to Markdown converter, enough for the
// Mosquitto man pages: xi:include and DOCTYPE entities are resolved
// from the loaded files, the rest is a tree walk.

type dbNode struct {
	name   string // "" for text
	attr   map[string]string
	text   string
	kids   []*dbNode
	parent *dbNode
}

var (
	dbDoctypeRe = regexp.MustCompile(`(?s)<!DOCTYPE[^\[>]*\[(.*?)\]>`)
	dbEntityRe  = regexp.MustCompile(`<!ENTITY\s+(\S+)\s+(SYSTEM\s+)?"([^"]*)"`)
	dbDeclRe    = regexp.MustCompile(`<\?xml[^>]*\?>`)
)

// docbookToMarkdown converts one refentry document. read returns the
// content of a file path relative to the document directory.
func docbookToMarkdown(src string, read func(href string) string) (string, error) {
	root, err := parseDocBook(src, read, 0)
	if err != nil {
		return "", err
	}
	r := &dbRender{}
	return strings.TrimSpace(r.refentry(root)) + "\n", nil
}

func parseDocBook(src string, read func(string) string, depth int) (*dbNode, error) {
	var entities [][2]string
	if m := dbDoctypeRe.FindStringSubmatch(src); m != nil {
		for _, e := range dbEntityRe.FindAllStringSubmatch(m[1], -1) {
			val := e[3]
			if e[2] != "" {
				val = dbDeclRe.ReplaceAllString(read(val), "")
			}
			entities = append(entities, [2]string{"&" + e[1] + ";", val})
		}
		src = dbDoctypeRe.ReplaceAllString(src, "")
	}
	// External entities first: their text may use the string ones.
	for i := 0; i < 2; i++ {
		for _, e := range entities {
			src = strings.ReplaceAll(src, e[0], e[1])
		}
	}
	dec := xml.NewDecoder(strings.NewReader(src))
	root := &dbNode{}
	cur := root
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &dbNode{name: t.Name.Local, attr: map[string]string{}, parent: cur}
			for _, a := range t.Attr {
				n.attr[a.Name.Local] = a.Value
			}
			cur.kids = append(cur.kids, n)
			cur = n
		case xml.EndElement:
			cur = cur.parent
			if n := cur.kids[len(cur.kids)-1]; n.name == "include" {
				cur.kids = append(cur.kids[:len(cur.kids)-1], dbInclude(n, read, depth)...)
			}
		case xml.CharData:
			cur.kids = append(cur.kids, &dbNode{text: string(t)})
		}
	}
	if len(root.kids) == 0 {
		return nil, fmt.Errorf("empty document")
	}
	for _, k := range root.kids {
		if k.name != "" {
			return k, nil
		}
	}
	return nil, fmt.Errorf("no root element")
}

// dbInclude returns the children of an included file, or nothing when
// it cannot be read.
func dbInclude(n *dbNode, read func(string) string, depth int) []*dbNode {
	body := dbDeclRe.ReplaceAllString(read(n.attr["href"]), "")
	if body == "" || depth > 2 {
		return nil
	}
	w, err := parseDocBook("<wrap>"+body+"</wrap>", read, depth+1)
	if err != nil {
		return nil
	}
	return w.kids
}

type dbRender struct {
	level int // heading level of the current section
	lists int // enclosing list items and variable list entries
}

var dbBlock = map[string]bool{
	"refsect1": true, "refsect2": true, "refsect3": true, "refsynopsisdiv": true,
	"para": true, "programlisting": true, "variablelist": true, "itemizedlist": true,
	"simplelist": true, "informaltable": true, "note": true, "important": true,
	"warning": true, "tip": true, "caution": true, "example": true,
}

func (r *dbRender) refentry(n *dbNode) string {
	var title, purpose string
	var body strings.Builder
	for _, k := range n.kids {
		switch k.name {
		case "refmeta":
			title = dbText(dbFind(k, "refentrytitle")) + "(" + dbText(dbFind(k, "manvolnum")) + ")"
		case "refnamediv":
			purpose = r.inline(dbFind(k, "refpurpose"))
			if title == "()" || title == "" {
				title = dbText(dbFind(k, "refname"))
			}
		default:
			body.WriteString(r.blocks([]*dbNode{k}))
		}
	}
	head := "# " + title + "\n\n"
	if purpose = dbSpace(purpose); purpose != "" {
		head += purpose + "\n\n"
	}
	return head + body.String()
}

// blocks renders child nodes: runs of inline content become paragraphs,
// block elements are rendered on their own.
func (r *dbRender) blocks(kids []*dbNode) string {
	var out, run strings.Builder
	flush := func() {
		if p := dbSpace(run.String()); p != "" {
			out.WriteString(p + "\n\n")
		}
		run.Reset()
	}
	for _, k := range kids {
		if !dbBlock[k.name] {
			run.WriteString(r.inline(k))
			continue
		}
		flush()
		out.WriteString(r.block(k))
	}
	flush()
	return out.String()
}

func (r *dbRender) block(n *dbNode) string {
	switch n.name {
	case "refsect1", "refsect2", "refsect3":
		saved := r.level
		r.level = int(n.name[len(n.name)-1]-'0') + 1
		title := dbSpace(r.inline(dbFind(n, "title")))
		s := strings.Repeat("#", r.level) + " " + title + "\n\n" + r.blocks(dbWithout(n.kids, "title"))
		r.level = saved
		return s
	case "refsynopsisdiv":
		var lines []string
		for _, k := range n.kids {
			if k.name == "cmdsynopsis" {
				lines = append(lines, r.synopsis(k))
			}
		}
		return "## Synopsis\n\n```\n" + strings.Join(lines, "\n") + "\n```\n\n"
	case "para":
		return r.blocks(n.kids)
	case "programlisting":
		return "```" + n.attr["language"] + "\n" + strings.Trim(dbRaw(n), "\n") + "\n```\n\n"
	case "variablelist":
		return r.variablelist(n)
	case "itemizedlist":
		var out strings.Builder
		for _, li := range dbFindAll(n, "listitem") {
			out.WriteString(r.item("- ", li))
		}
		return out.String() + "\n"
	case "simplelist":
		var out strings.Builder
		for _, m := range dbFindAll(n, "member") {
			out.WriteString("- " + dbSpace(r.inline(m)) + "\n")
		}
		return out.String() + "\n"
	case "informaltable":
		return r.table(n)
	case "example":
		s := ""
		if t := dbFind(n, "title"); t != nil {
			s = "**Example: " + dbSpace(r.inline(t)) + "**\n\n"
		}
		return s + r.blocks(dbWithout(n.kids, "title"))
	}
	// note, important, warning, tip, caution
	body := r.blocks(n.kids)
	return "**" + strings.ToUpper(n.name[:1]) + n.name[1:] + ":** " + body
}

func (r *dbRender) item(marker string, li *dbNode) string {
	r.lists++
	body := strings.TrimRight(r.blocks(li.kids), "\n")
	r.lists--
	var out []string
	for i, l := range strings.Split(body, "\n") {
		switch {
		case i == 0:
			l = marker + l
		case l != "":
			l = strings.Repeat(" ", len(marker)) + l
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n") + "\n"
}

func (r *dbRender) variablelist(n *dbNode) string {
	var out strings.Builder
	for _, e := range n.kids {
		if e.name != "varlistentry" {
			continue
		}
		var terms []string
		for _, t := range dbFindAll(e, "term") {
			terms = append(terms, dbSpace(r.inline(t)))
		}
		term := strings.Join(terms, ", ")
		if r.lists == 0 {
			// Headings deeper than h3 are not indexed, so options stay at h3.
			out.WriteString(strings.Repeat("#", min(r.level+1, 3)) + " " + term + "\n\n")
		} else {
			out.WriteString("**" + term + "**\n\n")
		}
		r.lists++
		if li := dbFind(e, "listitem"); li != nil {
			out.WriteString(r.blocks(li.kids))
		}
		r.lists--
	}
	return out.String()
}

func (r *dbRender) table(n *dbNode) string {
	var rows []string
	cells := func(row *dbNode) string {
		var c []string
		for _, e := range dbFindAll(row, "entry") {
			c = append(c, strings.ReplaceAll(dbSpace(r.inline(e)), "|", `\|`))
		}
		return "| " + strings.Join(c, " | ") + " |"
	}
	for _, sect := range dbFindAll(dbFind(n, "tgroup"), "thead", "tbody") {
		for _, row := range dbFindAll(sect, "row") {
			rows = append(rows, cells(row))
			if sect.name == "thead" {
				rows = append(rows, "|"+strings.Repeat(" --- |", strings.Count(rows[0], "|")-1))
			}
		}
	}
	return strings.Join(rows, "\n") + "\n\n"
}

// synopsis renders a cmdsynopsis as one or more plain-text lines.
func (r *dbRender) synopsis(n *dbNode) string {
	return strings.TrimSpace(dbSyn(n))
}

func dbSyn(n *dbNode) string {
	var parts []string
	for _, k := range n.kids {
		switch k.name {
		case "":
			if s := dbSpace(k.text); s != "" {
				parts = append(parts, s)
			}
		case "sbr":
			parts = append(parts, "\n   ")
		case "group":
			var alts []string
			for _, a := range k.kids {
				if a.name != "" {
					alts = append(alts, strings.TrimSpace(dbSyn(a)))
				}
			}
			parts = append(parts, dbChoice(k.attr["choice"], strings.Join(alts, " | "), k.attr["rep"]))
		case "arg":
			parts = append(parts, dbChoice(k.attr["choice"], strings.TrimSpace(dbSyn(k)), k.attr["rep"]))
		default:
			parts = append(parts, dbText(k))
		}
	}
	s := strings.Join(parts, " ")
	return strings.ReplaceAll(s, " \n   ", "\n   ")
}

func dbChoice(choice, s, rep string) string {
	switch choice {
	case "plain":
	case "req":
		s = "{ " + s + " }"
	default:
		s = "[" + s + "]"
	}
	if rep == "repeat" {
		s += "..."
	}
	return s
}

var dbEscapeRe = strings.NewReplacer(`\`, `\\`, "`", "\\`", "*", `\*`, "_", `\_`, "<", `\<`, ">", `\>`, "[", `\[`, "]", `\]`)

// inline renders a node as inline Markdown.
func (r *dbRender) inline(n *dbNode) string {
	if n == nil {
		return ""
	}
	if n.name == "" {
		return dbEscapeRe.Replace(n.text)
	}
	kids := func() string {
		var s strings.Builder
		for _, k := range n.kids {
			s.WriteString(r.inline(k))
		}
		return s.String()
	}
	switch n.name {
	case "option", "command", "literal", "filename", "code", "envar", "computeroutput", "userinput":
		return dbMark(dbText(n), "`")
	case "replaceable":
		return dbMark(kids(), "*")
	case "emphasis":
		return dbMark(kids(), "*")
	case "optional":
		return "[" + kids() + "]"
	case "ulink", "link":
		href := n.attr["url"] + n.attr["href"]
		text := kids()
		if href == "" {
			return text
		}
		if !strings.Contains(href, "://") {
			// Relative links point to sibling man pages (name-vol.html).
			href = "https://mosquitto.org/man/" + strings.TrimSuffix(href, ".html")
		}
		if strings.TrimSpace(text) == "" {
			text = href
		}
		return "[" + strings.TrimSpace(text) + "](" + href + ")"
	case "citerefentry":
		name, vol := dbText(dbFind(n, "refentrytitle")), dbText(dbFind(n, "manvolnum"))
		text := name + "(" + vol + ")"
		if l := dbFind(dbFind(n, "refentrytitle"), "link"); l != nil && l.attr["href"] != "" {
			return "[" + dbEscapeRe.Replace(text) + "](https://mosquitto.org/man/" + strings.TrimSuffix(l.attr["href"], ".html") + ")"
		}
		return dbEscapeRe.Replace(text)
	case "email", "uri":
		return dbMark(dbText(n), "`")
	}
	return kids()
}

// dbMark wraps s in marks, keeping surrounding spaces outside.
func dbMark(s, mark string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return s
	}
	lead, trail := "", ""
	if strings.TrimLeft(s, " \t\n") != s {
		lead = " "
	}
	if strings.TrimRight(s, " \t\n") != s {
		trail = " "
	}
	return lead + mark + dbSpace(t) + mark + trail
}

// dbSpace collapses whitespace runs into single spaces.
func dbSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

// dbText returns the plain text of a node with collapsed whitespace.
func dbText(n *dbNode) string { return dbSpace(dbRaw(n)) }

// dbRaw returns the plain text of a node as written.
func dbRaw(n *dbNode) string {
	if n == nil {
		return ""
	}
	if n.name == "" {
		return n.text
	}
	var s strings.Builder
	for _, k := range n.kids {
		s.WriteString(dbRaw(k))
	}
	return s.String()
}

func dbFind(n *dbNode, name string) *dbNode {
	if n == nil {
		return nil
	}
	for _, k := range n.kids {
		if k.name == name {
			return k
		}
	}
	return nil
}

func dbFindAll(n *dbNode, names ...string) []*dbNode {
	var out []*dbNode
	if n == nil {
		return nil
	}
	for _, k := range n.kids {
		for _, name := range names {
			if k.name == name {
				out = append(out, k)
			}
		}
	}
	return out
}

func dbWithout(kids []*dbNode, name string) []*dbNode {
	var out []*dbNode
	for _, k := range kids {
		if k.name != name {
			out = append(out, k)
		}
	}
	return out
}

var dbSlugRe = regexp.MustCompile(`(?m)^\.\. slug:\s*(\S+)`)

// mosquittoPrepare converts man/*.xml pages to man/<slug>.md file keys.
func mosquittoPrepare(files map[string]string) {
	for f, src := range files {
		if path.Dir(f) != "man" || path.Ext(f) != ".xml" {
			continue
		}
		base := strings.TrimSuffix(path.Base(f), ".xml")
		slug := strings.ReplaceAll(base, ".", "-")
		if m := dbSlugRe.FindStringSubmatch(files["man/"+base+".meta"]); m != nil {
			slug = m[1]
		}
		md, err := docbookToMarkdown(src, func(href string) string { return files[path.Join("man", href)] })
		if err != nil {
			fmt.Fprintf(os.Stderr, "mosquitto: %s: %v\n", f, err)
			continue
		}
		files["man/"+slug+".md"] = md
	}
}
