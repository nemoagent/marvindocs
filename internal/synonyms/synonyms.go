// Package synonyms holds a thesaurus of search synonyms used to expand
// queries: "k8s" also finds "kubernetes", "regex" finds "regular
// expression".
//
// The source format follows Terraphim and Logseq knowledge graphs: one
// Markdown file per concept. The concept is the first "# heading" of
// the file, or the file name without ".md" when there is none, and a
// line "synonyms:: a, b, c" lists its other names. Every name of a
// concept is a synonym of every other one. Matching is case-insensitive
// and on whole words; a name may have several words.
package synonyms

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
)

//go:embed builtin/*.md
var builtinFS embed.FS

// Thesaurus maps a term to its synonyms. A nil *Thesaurus has none.
type Thesaurus struct {
	syn      map[string][]string // lower-case term -> its synonyms
	maxWords int                 // words in the longest term
}

// Builtin returns the thesaurus shipped in the binary.
func Builtin() (*Thesaurus, error) {
	t := &Thesaurus{}
	if err := t.load(builtinFS, "builtin"); err != nil {
		return nil, fmt.Errorf("builtin synonyms: %w", err)
	}
	return t, nil
}

// LoadDir adds the concepts of the *.md files in dir to t.
func (t *Thesaurus) LoadDir(dir string) error {
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("synonyms: %w", err)
	}
	if err := t.load(os.DirFS(dir), "."); err != nil {
		return fmt.Errorf("synonyms %s: %w", dir, err)
	}
	return nil
}

func (t *Thesaurus) load(fsys fs.FS, dir string) error {
	files, err := fs.Glob(fsys, path.Join(dir, "*.md"))
	if err != nil {
		return err
	}
	for _, name := range files {
		f, err := fsys.Open(name)
		if err != nil {
			return err
		}
		terms, err := parse(f, strings.TrimSuffix(path.Base(name), ".md"))
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		t.add(terms)
	}
	return nil
}

// parse reads one concept file: the concept first, then its synonyms.
func parse(f fs.File, stem string) ([]string, error) {
	concept := stem
	var terms []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if h, ok := strings.CutPrefix(line, "# "); ok && len(terms) == 0 && concept == stem {
			concept = h
			continue
		}
		key, val, ok := strings.Cut(line, "::")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "synonyms") {
			continue
		}
		terms = append(terms, strings.Split(val, ",")...)
	}
	return append([]string{concept}, terms...), sc.Err()
}

// add makes every term a synonym of every other one.
func (t *Thesaurus) add(terms []string) {
	if t.syn == nil {
		t.syn = make(map[string][]string)
	}
	var norm []string
	for _, s := range terms {
		s = strings.ToLower(strings.Join(strings.Fields(s), " "))
		if s != "" && !contains(norm, s) {
			norm = append(norm, s)
		}
	}
	for _, a := range norm {
		t.maxWords = max(t.maxWords, strings.Count(a, " ")+1)
		for _, b := range norm {
			if b != a && !contains(t.syn[a], b) {
				t.syn[a] = append(t.syn[a], b)
			}
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Lookup returns the synonyms of a single term, or nil.
func (t *Thesaurus) Lookup(term string) []string {
	if t == nil {
		return nil
	}
	return t.syn[strings.ToLower(term)]
}

// Match finds the longest term at the start of words and returns how
// many words it spans and its synonyms; n is 0 when nothing matches.
func (t *Thesaurus) Match(words []string) (n int, syns []string) {
	if t == nil {
		return 0, nil
	}
	for n = min(t.maxWords, len(words)); n > 0; n-- {
		if s, ok := t.syn[strings.ToLower(strings.Join(words[:n], " "))]; ok {
			return n, s
		}
	}
	return 0, nil
}
