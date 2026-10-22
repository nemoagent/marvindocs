package main

import (
	"archive/zip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

func TestParsePage(t *testing.T) {
	base, _ := url.Parse("https://docs.metasploit.com/")
	u, _ := url.Parse("https://docs.metasploit.com/docs/using/start.html")
	body, title, hs, err := parsePage([]byte(`<title>Using | Metasploit</title><nav>Navigation<svg><title>Document</title></svg></nav><div id="main-content"><h1 id="start"><a class="anchor-heading">anchor</a>Using <code>msfconsole</code></h1><pre><code>help &lt;command&gt;</code></pre><a href="https://rapid7.github.io/metasploit-framework/docs/other.html#part">Next</a><a href="/">Home</a><a href="/missing.html">Missing</a><img src="/images/example.png"><footer>Footer</footer><script>bad()</script></div>`), u, base, "docs/using/start.html", map[string]bool{"index": true, "docs/other.html": true})
	if err != nil {
		t.Fatal(err)
	}
	if title != "Using" || len(hs) != 1 || hs[0].title != "Using msfconsole" || hs[0].id != "start" {
		t.Fatalf("title/headings: %q %+v", title, hs)
	}
	for _, want := range []string{`href="../other.html#part"`, `href="../../index"`, `href="https://docs.metasploit.com/missing.html"`, `src="https://docs.metasploit.com/images/example.png"`, `help &lt;command&gt;`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in %s", want, body)
		}
	}
	for _, unwanted := range []string{"Navigation", "Footer", "bad()", "anchor-heading"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("retained %s", unwanted)
		}
	}
}

func TestRun(t *testing.T) {
	fail := false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap.xml":
			fmt.Fprint(w, `<urlset><url><loc>https://rapid7.github.io/metasploit-framework/docs/using.html</loc></url><url><loc>https://rapid7.github.io/metasploit-framework/docs/using.html</loc></url><url><loc>https://example.com/docs/ignored.html</loc></url></urlset>`)
		case "/", "/docs/using.html":
			if fail {
				http.Error(w, "unavailable", 503)
				return
			}
			fmt.Fprint(w, `<title>Using | Metasploit</title><div id="main-content"><h1 id="usage">Usage</h1><p>Console documentation</p></div>`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	f := devdocsfmt.Flags{Source: s.URL, Out: filepath.Join(t.TempDir(), "metasploit")}
	if err := run(f); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(filepath.Join(f.Out, "db.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	if len(z.File) != 2 {
		t.Fatalf("pages: %d", len(z.File))
	}
	before, err := os.ReadFile(filepath.Join(f.Out, "db.zip"))
	if err != nil {
		t.Fatal(err)
	}
	fail = true
	f.Force = true
	if err := run(f); err == nil {
		t.Fatal("expected fetch error")
	}
	after, err := os.ReadFile(filepath.Join(f.Out, "db.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("failed fetch changed existing archive")
	}
}

func TestMissingContent(t *testing.T) {
	u, _ := url.Parse("https://docs.metasploit.com/")
	for _, input := range []string{`<title>Error</title><p>Not found</p>`, `<title>Empty</title><div id="main-content"></div>`} {
		if _, _, _, err := parsePage([]byte(input), u, u, "index", nil); err == nil {
			t.Fatal("expected error")
		}
	}
}
