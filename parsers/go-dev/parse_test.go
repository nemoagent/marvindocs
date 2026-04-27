package main

import "testing"

func TestExtractArticle(t *testing.T) {
	html := `<html><head><title>x</title></head><body>
<header>nav</header>
<article>
  <h1>Title</h1>
  <h2 id="intro">Introduction</h2>
  <p>hello world</p>
  <h2 id="tools">Tools</h2>
  <h3 id="vet">Vet</h3>
  <p>more</p>
</article>
<footer>foot</footer>
</body></html>`

	body, hs, err := extractArticle(html)
	if err != nil {
		t.Fatal(err)
	}
	if body == "" {
		t.Fatal("empty body")
	}
	if len(hs) != 3 {
		t.Fatalf("want 3 headings, got %d (%+v)", len(hs), hs)
	}
	want := []extractedHeading{
		{2, "intro", "Introduction"},
		{2, "tools", "Tools"},
		{3, "vet", "Vet"},
	}
	for i, h := range hs {
		if h != want[i] {
			t.Errorf("heading %d: got %+v, want %+v", i, h, want[i])
		}
	}
}

func TestExtractArticleFallbackToBody(t *testing.T) {
	html := `<html><body><h2 id="x">X</h2><p>p</p></body></html>`
	body, hs, err := extractArticle(html)
	if err != nil {
		t.Fatal(err)
	}
	if body == "" {
		t.Fatal("expected non-empty body fallback")
	}
	if len(hs) != 1 || hs[0].id != "x" {
		t.Fatalf("want one h2#x, got %+v", hs)
	}
}

func TestTextContentNormalizesWhitespace(t *testing.T) {
	html := `<html><body><article><h2 id="a">  Hello   <em>World</em>
   </h2></article></body></html>`
	_, hs, err := extractArticle(html)
	if err != nil {
		t.Fatal(err)
	}
	if len(hs) != 1 || hs[0].text != "Hello World" {
		t.Fatalf("want 'Hello World', got %q", hs[0].text)
	}
}
