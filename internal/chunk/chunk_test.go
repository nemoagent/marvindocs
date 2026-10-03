package chunk

import (
	"strings"
	"testing"
)

func TestSplitByHeadings(t *testing.T) {
	md := "intro text\n\n# Title\n\nbody one\n\n## Sub\n\n```\n# not a heading\n```\nbody two\n"
	got := Split(md)
	if len(got) != 3 {
		t.Fatalf("got %d sections, want 3: %+v", len(got), got)
	}
	if got[0].Heading != "" || got[0].Text != "intro text" {
		t.Errorf("preamble = %+v", got[0])
	}
	if got[1].Heading != "Title" || !strings.HasPrefix(got[1].Text, "# Title") {
		t.Errorf("section 1 = %+v", got[1])
	}
	if got[2].Heading != "Sub" || !strings.Contains(got[2].Text, "# not a heading") {
		t.Errorf("fenced heading must stay inside section: %+v", got[2])
	}
}

func TestSplitLongSection(t *testing.T) {
	para := strings.Repeat("word ", 1000) // ~5000 chars
	md := "# Big\n\n" + para + "\n\n" + para + "\n\n" + para
	got := Split(md)
	if len(got) != 3 {
		t.Fatalf("got %d pieces, want 3", len(got))
	}
	for _, s := range got {
		if s.Heading != "Big" {
			t.Errorf("piece lost heading: %q", s.Heading)
		}
		if len(s.Text) > maxSectionChars {
			t.Errorf("piece too long: %d", len(s.Text))
		}
	}
}

func TestScore(t *testing.T) {
	s := Section{Heading: "JSON Types", Text: "# JSON Types\n\nThe jsonb type stores binary data."}
	terms := Terms("jsonb type")
	// "jsonb": body only; "type": body + heading ("Types" prefix).
	if got := Score(s, terms); got != 3 {
		t.Errorf("score = %d, want 3", got)
	}
	if got := Score(s, Terms("postgres")); got != 0 {
		t.Errorf("unrelated score = %d, want 0", got)
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
