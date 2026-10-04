package docstore

import (
	"strings"
	"testing"
)

func TestToPlainText(t *testing.T) {
	in := `<h1>Hello</h1><p>world <code>x</code>!</p><script>var a=1;</script>`
	got := ToPlainText(in)
	want := "Hello world x !"
	// allow minor spacing differences
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "world") ||
		!strings.Contains(got, "x") || strings.Contains(got, "var a=1") {
		t.Fatalf("ToPlainText returned %q, want something like %q", got, want)
	}
}

func TestToMarkdown(t *testing.T) {
	in := `<h1>Hi</h1><p><strong>bold</strong> and <em>italic</em></p>`
	md, err := ToMarkdown(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "# Hi") || !strings.Contains(md, "**bold**") {
		t.Fatalf("unexpected markdown: %q", md)
	}
}

func TestToMarkdownStripsDataURIs(t *testing.T) {
	in := `<p>see <img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUg==" alt="plot"> and <a href="http://x/y.png">y</a></p>`
	md, err := ToMarkdown(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(md, "iVBORw0") || !strings.Contains(md, "http://x/y.png") {
		t.Fatalf("unexpected markdown: %q", md)
	}
}
