package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/piligrim/marvindocs/parsers/devdocsfmt"
)

var stdPackages = []string{
	"archive/tar", "archive/zip",
	"bufio", "bytes",
	"cmp", "compress/bzip2", "compress/flate", "compress/gzip", "compress/lzw", "compress/zlib", "container/heap", "container/list", "container/ring", "context", "crypto", "crypto/aes", "crypto/cipher", "crypto/des", "crypto/dsa", "crypto/ecdh", "crypto/ecdsa", "crypto/ed25519", "crypto/elliptic", "crypto/hmac", "crypto/md5", "crypto/rand", "crypto/rc4", "crypto/rsa", "crypto/sha1", "crypto/sha256", "crypto/sha512", "crypto/subtle", "crypto/tls", "crypto/x509", "crypto/x509/pkix",
	"database/sql", "database/sql/driver", "debug/buildinfo", "debug/dwarf", "debug/elf", "debug/gosym", "debug/macho", "debug/pe", "debug/plan9obj", "embed", "encoding", "encoding/ascii85", "encoding/asn1", "encoding/base32", "encoding/base64", "encoding/binary", "encoding/csv", "encoding/gob", "encoding/hex", "encoding/json", "encoding/pem", "encoding/xml", "errors", "expvar",
	"flag", "fmt",
	"go/ast", "go/build", "go/build/constraint", "go/constant", "go/doc", "go/doc/comment", "go/format", "go/importer", "go/parser", "go/printer", "go/scanner", "go/token", "go/types",
	"hash", "hash/adler32", "hash/crc32", "hash/crc64", "hash/fnv", "html", "html/template",
	"image", "image/color", "image/color/palette", "image/draw", "image/gif", "image/jpeg", "image/png", "index/suffixarray", "io", "io/fs", "io/ioutil", "iter",
	"log", "log/slog",
	"maps", "math", "math/big", "math/bits", "math/cmplx", "math/rand", "math/rand/v2", "mime", "mime/multipart", "mime/quotedprintable",
	"net", "net/http", "net/http/cgi", "net/http/cookiejar", "net/http/fcgi", "net/http/httptest", "net/http/httptrace", "net/http/httputil", "net/http/pprof", "net/mail", "net/netip", "net/rpc", "net/rpc/jsonrpc", "net/smtp", "net/textproto", "net/url",
	"os", "os/exec", "os/signal", "os/user",
	"path", "path/filepath", "plugin",
	"reflect", "regexp", "regexp/syntax", "runtime", "runtime/cgo", "runtime/coverage", "runtime/debug", "runtime/metrics", "runtime/pprof", "runtime/race", "runtime/trace",
	"slices", "sort", "strconv", "strings", "structs", "sync", "sync/atomic", "syscall",
	"testing", "testing/fstest", "testing/iotest", "testing/quick", "testing/slogtest",
	"text/scanner", "text/tabwriter", "text/template", "text/template/parse", "time", "time/tzdata",
	"unicode", "unicode/utf16", "unicode/utf8", "unique", "unsafe",
}

var sectionByPrefix = []struct {
	prefix string
	typ    string
}{
	{"archive/", "Archive"},
	{"bufio", "I/O"},
	{"bytes", "I/O"},
	{"cmp", "Utilities"},
	{"compress/", "Compression"},
	{"container/", "Containers"},
	{"context", "Concurrency"},
	{"crypto", "Crypto"},
	{"database/", "Database"},
	{"debug/", "Debug"},
	{"embed", "Runtime"},
	{"encoding", "Encoding"},
	{"errors", "Errors"},
	{"expvar", "Runtime"},
	{"flag", "CLI"},
	{"fmt", "Formatting"},
	{"go/", "Go Tools"},
	{"hash", "Crypto"},
	{"html", "Web"},
	{"image", "Image"},
	{"index/", "Algorithms"},
	{"io", "I/O"},
	{"iter", "Utilities"},
	{"log", "Logging"},
	{"maps", "Utilities"},
	{"math", "Math"},
	{"mime", "Web"},
	{"net", "Networking"},
	{"os", "OS"},
	{"path", "Filesystem"},
	{"plugin", "Runtime"},
	{"reflect", "Reflection"},
	{"regexp", "Text"},
	{"runtime", "Runtime"},
	{"slices", "Utilities"},
	{"sort", "Utilities"},
	{"strconv", "Text"},
	{"strings", "Text"},
	{"structs", "Utilities"},
	{"sync", "Concurrency"},
	{"syscall", "OS"},
	{"testing", "Testing"},
	{"text/", "Text"},
	{"time", "Time"},
	{"unicode", "Text"},
	{"unique", "Utilities"},
	{"unsafe", "Runtime"},
}

func run(flags devdocsfmt.Flags, image string) error {
	doc := devdocsfmt.Doc{
		Name:        "Go Standard Library",
		Slug:        "go-std~" + flags.Version,
		Type:        "go",
		Version:     flags.Version,
		Release:     flags.Release,
		Mtime:       time.Now().Unix(),
		Attribution: "Generated from go doc inside Docker image " + image,
	}
	w := devdocsfmt.NewWriter(doc)

	indexBody := buildIndexPage(flags.Version, image)
	w.AddPage("index", indexBody)
	w.AddEntry("Standard library", "index", "Index")

	for _, pkg := range stdPackages {
		body, symbols, err := packageDoc(image, pkg)
		if err == errSkipPkg {
			fmt.Fprintln(os.Stderr, "  skip", pkg, "(not in this Go version)")
			continue
		}
		if err != nil {
			return err
		}
		path := pkg
		typ := packageType(pkg)
		w.AddPage(path, body)
		w.AddEntry(pkg, path, typ)
		for _, s := range symbols {
			anchor := path + "#" + anchorize(s)
			w.AddEntry(pkg+"."+s, anchor, typ)
		}
	}
	return w.Flush(flags.Out, flags.Force)
}

func packageDoc(image, pkg string) (string, []string, error) {
	full, err := runGoDoc(image, "-all", pkg)
	if err != nil {
		if isMissingPkgErr(err) {
			return "", nil, errSkipPkg
		}
		return "", nil, fmt.Errorf("go doc -all %s: %w", pkg, err)
	}
	head, err := runGoDoc(image, pkg)
	if err != nil {
		if isMissingPkgErr(err) {
			return "", nil, errSkipPkg
		}
		return "", nil, fmt.Errorf("go doc %s: %w", pkg, err)
	}
	symbols := parseSymbols(head)
	return toHTML(pkg, full), symbols, nil
}

var errSkipPkg = fmt.Errorf("package not available in this Go version")

func isMissingPkgErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no buildable Go source files") ||
		strings.Contains(msg, "no such package") ||
		strings.Contains(msg, "cannot find package") ||
		strings.Contains(msg, "is not in std")
}

func runGoDoc(image string, args ...string) (string, error) {
	cmdArgs := append([]string{"run", "--rm", image, "go", "doc"}, args...)
	cmd := exec.Command("docker", cmdArgs...)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("docker %s: %s", strings.Join(cmdArgs, " "), msg)
	}
	return out.String(), nil
}

var symbolLineRe = regexp.MustCompile(`^[A-Z][A-Za-z0-9_]*`)

func parseSymbols(pkgDoc string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, line := range strings.Split(pkgDoc, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "func ") {
			rest := strings.TrimPrefix(line, "func ")
			if strings.HasPrefix(rest, "(") {
				continue
			}
			name := symbolLineRe.FindString(rest)
			if name != "" {
				if _, ok := seen[name]; !ok {
					seen[name] = struct{}{}
					out = append(out, name)
				}
			}
			continue
		}
		if strings.HasPrefix(line, "type ") {
			rest := strings.TrimPrefix(line, "type ")
			name := symbolLineRe.FindString(rest)
			if name != "" {
				if _, ok := seen[name]; !ok {
					seen[name] = struct{}{}
					out = append(out, name)
				}
			}
			continue
		}
		if strings.HasPrefix(line, "var ") {
			rest := strings.TrimPrefix(line, "var ")
			name := symbolLineRe.FindString(rest)
			if name != "" {
				if _, ok := seen[name]; !ok {
					seen[name] = struct{}{}
					out = append(out, name)
				}
			}
			continue
		}
		if strings.HasPrefix(line, "const ") {
			rest := strings.TrimPrefix(line, "const ")
			name := symbolLineRe.FindString(rest)
			if name != "" {
				if _, ok := seen[name]; !ok {
					seen[name] = struct{}{}
					out = append(out, name)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

func toHTML(pkg, text string) string {
	var b strings.Builder
	b.WriteString("<h1>")
	b.WriteString(escapeHTML(pkg))
	b.WriteString("</h1><pre><code>")
	b.WriteString(escapeHTML(text))
	b.WriteString("</code></pre>")
	return b.String()
}

func escapeHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func packageType(pkg string) string {
	for _, m := range sectionByPrefix {
		if pkg == m.prefix || strings.HasPrefix(pkg, m.prefix) {
			return m.typ
		}
	}
	return "Standard Library"
}

func anchorize(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func buildIndexPage(version, image string) string {
	groups := map[string][]string{}
	for _, pkg := range stdPackages {
		typ := packageType(pkg)
		groups[typ] = append(groups[typ], pkg)
	}
	var sections []string
	for k := range groups {
		sections = append(sections, k)
	}
	sort.Strings(sections)

	var b strings.Builder
	fmt.Fprintf(&b, "<h1>Go %s Standard Library</h1>", escapeHTML(version))
	fmt.Fprintf(&b, "<p>Generated from <code>go doc</code> inside Docker image <code>%s</code>.</p>", escapeHTML(image))
	for _, section := range sections {
		pkgs := groups[section]
		sort.Strings(pkgs)
		fmt.Fprintf(&b, "<h2>%s</h2><ul>", escapeHTML(section))
		for _, pkg := range pkgs {
			fmt.Fprintf(&b, `<li><a href="%s">%s</a></li>`, escapeHTML(pkg), escapeHTML(pkg))
		}
		b.WriteString("</ul>")
	}
	return b.String()
}
