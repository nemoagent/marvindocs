package chunk

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// texts returns the chunk texts re-sliced from md.
func texts(md string, cs []Chunk) []string {
	var out []string
	for _, c := range cs {
		out = append(out, md[c.Off:c.Off+c.Len])
	}
	return out
}

func TestChunksOffsetsAndHeadings(t *testing.T) {
	body := strings.TrimSpace(strings.Repeat("text ", 50)) // above minChunkChars
	md := "intro " + body + "\n\n# Title\n\nbody one " + body + "\n\n## Sub\n\n```\n# not a heading\n```\nbody two " + body + "\n"
	got := Chunks(md)
	want := []string{
		"intro " + body,
		"# Title\n\nbody one " + body,
		"## Sub\n\n```\n# not a heading\n```\nbody two " + body,
	}
	if g := texts(md, got); strings.Join(g, "|") != strings.Join(want, "|") {
		t.Fatalf("chunks = %q, want %q", g, want)
	}
	for k, h := range []string{"", "Title", "Sub"} {
		if got[k].Heading != h {
			t.Errorf("chunk %d heading = %q, want %q", k, got[k].Heading, h)
		}
	}
}

func TestChunksPacksParagraphs(t *testing.T) {
	para := strings.Repeat("word ", 100) // ~500 chars
	md := "# Big\n\n" + para + "\n\n" + para + "\n\n" + para
	got := Chunks(md)
	if len(got) != 2 {
		t.Fatalf("got %d chunks, want 2: %q", len(got), texts(md, got))
	}
	for _, c := range got {
		if c.Len > maxChunkChars {
			t.Errorf("chunk too long: %d", c.Len)
		}
		if c.Heading != "Big" {
			t.Errorf("chunk lost heading: %q", c.Heading)
		}
	}
}

func TestChunksHardCutsHugeParagraph(t *testing.T) {
	huge := strings.Repeat("\u00e9", 2*maxChunkChars) // 2 bytes per rune, 4*max bytes
	md := "# T\n\n" + huge
	got := Chunks(md)
	if len(got) < 3 {
		t.Fatalf("got %d chunks, want the paragraph cut", len(got))
	}
	if !strings.HasPrefix(md[got[0].Off:], "# T") {
		t.Fatalf("heading not in the first chunk: %+v", got[0])
	}
	// The heading line is merged into the first piece.
	pieces := texts(md, got)
	pieces[0] = strings.TrimPrefix(pieces[0], "# T\n\n")
	var joined strings.Builder
	for _, s := range pieces {
		if !utf8.ValidString(s) {
			t.Fatal("cut in the middle of a rune")
		}
		joined.WriteString(s)
	}
	if joined.String() != huge {
		t.Error("pieces do not cover the paragraph")
	}
}

func TestChunksMergesShort(t *testing.T) {
	body := strings.Repeat("text ", 60)
	md := "# Page\n\n## Usage\n\n### f()\n\n" + body + "\n\n## See also\n\nshort"
	got := Chunks(md)
	if len(got) != 1 {
		t.Fatalf("got %d chunks, want 1: %q", len(got), texts(md, got))
	}
	if got[0].Off != 0 || got[0].Off+got[0].Len != len(md) {
		t.Errorf("merged chunk does not span the page: %+v", got[0])
	}
	if got[0].Heading != "f()" {
		t.Errorf("heading = %q, want the heading of the body", got[0].Heading)
	}
}

func TestChunksDropsEmpty(t *testing.T) {
	if got := Chunks(" \n\n\t\n"); len(got) != 0 {
		t.Errorf("blank page gave chunks: %+v", got)
	}
}

func TestTitle(t *testing.T) {
	if got := Title("text\n```\n# no\n```\n## Real one\n# Later"); got != "Real one" {
		t.Errorf("title = %q", got)
	}
	if got := Title("no headings"); got != "" {
		t.Errorf("title = %q, want empty", got)
	}
}

func TestHeadingAt(t *testing.T) {
	md := "intro\n\n# A\n\ntext\n\n## B\n\nmore"
	if got := HeadingAt(md, 0); got != "" {
		t.Errorf("before first heading = %q", got)
	}
	if got := HeadingAt(md, strings.Index(md, "text")); got != "A" {
		t.Errorf("in A = %q", got)
	}
	if got := HeadingAt(md, strings.Index(md, "## B")); got != "B" {
		t.Errorf("at heading line = %q", got)
	}
}

func TestTruncate(t *testing.T) {
	s := strings.Repeat("abcd ", 100) // 500 chars ≈ 125 tokens
	if got := Truncate(s, 200); got != s {
		t.Error("text within budget must not change")
	}
	got := Truncate(s, 10)
	if Tokens(got) > 12 || !strings.HasSuffix(got, " …") {
		t.Errorf("truncated = %q (%d tokens)", got, Tokens(got))
	}
}
