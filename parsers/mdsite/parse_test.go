package main

import (
	"strings"
	"testing"
)

var testSite = Site{
	Home: "https://ex.com/docs/", BaseURL: "https://ex.com", URLPrefix: "/docs/",
	Roots:       []Root{{"content", ""}},
	IncludeDirs: []string{"inc"},
}

func TestCleanMarkdown(t *testing.T) {
	files := map[string]string{"inc/note.md": "---\ntitle: x\n---\nIncluded {{< b >}}text"}
	in := strings.Join([]string{
		"import Tabs from '@theme/Tabs'",
		`{{< include "note.md" >}}`,
		`!!! warning "Careful"`,
		"    indented body",
		"> [!NOTE]",
		"> note body",
		"<Tabs>",
		"```go",
		"<Keep> x",
		"```",
		"</Tabs>",
		`{{< tab name="Linux" >}}`,
	}, "\n")
	got := cleanMarkdown(testSite, in, files, 0)
	for _, want := range []string{"Included text", "**Careful**\n\nindented body", "> **NOTE:**", "```go\n<Keep> x\n```", "**Linux**"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, bad := range []string{"import Tabs", "<Tabs>", "</Tabs>", "!!!"} {
		if strings.Contains(got, bad) {
			t.Errorf("unexpected %q in:\n%s", bad, got)
		}
	}
}

func TestRewriteLinks(t *testing.T) {
	fileToPage := map[string]string{"content/a/b.md": "a/b", "content/c.md": "c"}
	known := map[string]bool{"a/b": true, "c": true}
	in := `<a href="../c.md#x">1</a> <a href="/c.md">2</a> <a href="https://ex.com/docs/a/b/">3</a> ` +
		`<a href="missing.md">4</a> <a href="/other/">5</a> <a href="#top">6</a> <img src="/docs/c/i.png">`
	want := `<a href="../c#x">1</a> <a href="../c">2</a> <a href="b">3</a> ` +
		`<a href="https://ex.com/docs/">4</a> <a href="https://ex.com/other">5</a> <a href="#top">6</a> <img src="https://ex.com/docs/c/i.png">`
	if got := rewriteLinks(testSite, in, "content/a/b.md", "a/b", fileToPage, known); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestMapPagesAndSections(t *testing.T) {
	site := testSite
	site.Skip = []string{"skip"}
	site.SectionDepth = 2
	files := map[string]string{
		"content/_index.md": "", "content/guide/_index.md": "", "content/guide/install.md": "",
		"content/guide/deep/x.md": "", "content/skip/y.md": "", "content/z.txt": "",
	}
	got := mapPages(site, files)
	want := map[string]string{
		"content/_index.md": "index", "content/guide/_index.md": "guide",
		"content/guide/install.md": "guide/install", "content/guide/deep/x.md": "guide/deep/x",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for f, p := range want {
		if got[f] != p {
			t.Errorf("%s: got %q want %q", f, got[f], p)
		}
	}
	titles := map[string]string{"index": "Home", "guide": "Overview", "guide/install": "Install", "guide/deep/x": "X"}
	cases := map[string]string{"index": "General", "guide/install": "Guide", "guide/deep/x": "Guide: Deep"}
	for p, w := range cases {
		if s := sectionFor(site, p, titles); s != w {
			t.Errorf("section %s: got %q want %q", p, s, w)
		}
	}
}

func TestSplitFrontMatter(t *testing.T) {
	title, body := splitFrontMatter("+++\nlinkTitle = \"Short\"\ntitle = \"Long\"\n+++\nbody")
	if title != "Long" || body != "body" {
		t.Fatalf("title=%q body=%q", title, body)
	}
}

func TestPodTemplate(t *testing.T) {
	in := `a<< if is_quadlet >>Q<< else >>C<< endif >>b<< "x" if is_quadlet else "y" >><<subcommand>>`
	if got := podTemplate(in, false); got != "aCby<<subcommand>>" {
		t.Errorf("podman: %q", got)
	}
	if got := podTemplate(in, true); got != "aQbx<<subcommand>>" {
		t.Errorf("quadlet: %q", got)
	}
}

func TestPodmanPrepare(t *testing.T) {
	const d = "docs/source/markdown/"
	files := map[string]string{
		d + "podman-pod-create.1.md.in": "% podman-pod-create 1\n\n@@option name\n",
		d + "options/name.md":           "####> used in\n#### **--name**\n<<fullsubcommand>> names the <<container|pod>>\n",
	}
	podmanPrepare(files)
	want := "# podman pod create\n#### **--name**\npodman pod create names the pod\n"
	if got := files[d+"podman-pod-create.1.md"]; got != want {
		t.Fatalf("got %q", got)
	}
	if podmanSection("markdown/podman-pod-create.1") != "podman pod" || podmanSection("markdown/podman-run.1") != "podman" {
		t.Fatal("podmanSection")
	}
}

func TestLatestVersion(t *testing.T) {
	files := map[string]string{
		"content/p/v2.4.9/content/docs/a.mdx":  "old",
		"content/p/v2.4.10/content/docs/a.mdx": "new",
		"content/p/v2.4.10/data/nav.json":      "",
	}
	latestVersion("p")(files)
	if len(files) != 1 || files["p/docs/a.mdx"] != "new" {
		t.Fatalf("got %v", files)
	}
}

func TestIstioClean(t *testing.T) {
	files := map[string]string{"content/en/boilerplates/bp.md": "---\n---\nBoiler"}
	got := istioClean("{{< text bash >}}\n$ ls\n{{< /text >}}\n{{< boilerplate bp >}}", files)
	if !strings.Contains(got, "```bash\n$ ls\n```") || !strings.Contains(got, "Boiler") {
		t.Fatalf("got %q", got)
	}
}

func TestMarkdownIncludeRaw(t *testing.T) {
	files := map[string]string{"docs/cm.yaml": "---\nkey: {{ value }}\n"}
	got := cleanMarkdown(testSite, "```yaml\n{!docs/cm.yaml!}\n```", files, 0)
	if want := "```yaml\n---\nkey: {{ value }}\n```"; !strings.Contains(got, want) {
		t.Errorf("want %q in:\n%s", want, got)
	}
}

func TestAtIncludeForms(t *testing.T) {
	files := map[string]string{"p.mdx": "PART"}
	for _, in := range []string{"@include 'p.mdx'", "`@include 'p.mdx'`", "- @include 'p.mdx'"} {
		if got := cleanMarkdown(testSite, in, files, 0); !strings.Contains(got, "PART") || strings.Contains(got, "@include") {
			t.Errorf("%q -> %q", in, got)
		}
	}
}
