package synonyms

import (
	"sort"
	"strings"
	"sync"
	"unicode"
)

// Mining abbreviation candidates out of documentation text: docs spell
// most abbreviations out once, as "Server-Side Rendering (SSR)" or
// "SSR (Server-Side Rendering)". Miner collects such pairs over many
// docs and keeps the ones that are frequent and unambiguous; the result
// is a table to review before it becomes synonym files.

const (
	minAbbrev   = 3 // shorter abbreviations (UI, EP, BD) are too ambiguous
	maxAbbrev   = 10
	maxLongWord = 8 // words in a spelled-out name
	minShare    = 0.6
)

// stopWords may be skipped by an abbreviation: "Internet of Things (IoT)"
// takes a letter from "of", "Bring Your Own Device" from none of them.
var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "as": true, "at": true, "by": true,
	"for": true, "in": true, "of": true, "on": true, "or": true, "the": true,
	"to": true, "with": true,
}

// FindAbbrevs calls fn for every "long name (ABBR)" and "ABBR (long
// name)" pair in text whose abbreviation is built from the leading
// letters of the name's words. abbr is upper-case, long is lower-case.
func FindAbbrevs(text string, fn func(abbr, long string)) {
	for i := 0; ; {
		open := strings.IndexByte(text[i:], '(')
		if open < 0 {
			return
		}
		open += i
		end := strings.IndexByte(text[open:], ')')
		if end < 0 {
			return
		}
		end += open
		inner := strings.TrimSpace(text[open+1 : end])
		before := lastWords(text[:open], maxLongWord)
		if abbr, ok := abbrev(inner); ok {
			// "Server-Side Rendering (SSR)": the shortest tail of the
			// preceding words that spells the abbreviation.
			for n := 1; n <= len(before); n++ {
				if long := before[len(before)-n:]; spells(abbr, long) {
					fn(abbr, strings.ToLower(strings.Join(long, " ")))
					break
				}
			}
		} else if len(before) > 0 {
			if abbr, ok := abbrev(before[len(before)-1]); ok {
				// "SSR (Server-Side Rendering)": the whole inner text.
				long := strings.Fields(inner)
				if len(long) <= maxLongWord && allClean(long) && spells(abbr, long) {
					fn(abbr, strings.ToLower(strings.Join(long, " ")))
				}
			}
		}
		i = open + 1
	}
}

// lastWords returns up to n trailing words of s, stopping at the first
// one that is not a plain word: punctuation ends a phrase.
func lastWords(s string, n int) []string {
	f := strings.Fields(s)
	k := len(f)
	for k > 0 && len(f)-k < n && clean(f[k-1]) {
		k--
	}
	return f[k:]
}

func allClean(words []string) bool {
	for _, w := range words {
		if !clean(w) {
			return false
		}
	}
	return len(words) > 0
}

// clean reports whether w is letters and inner hyphens only.
func clean(w string) bool {
	if w == "" || w[0] == '-' || w[len(w)-1] == '-' {
		return false
	}
	for _, r := range w {
		if r != '-' && (r > unicode.MaxASCII || !unicode.IsLetter(r)) {
			return false
		}
	}
	return true
}

// abbrev reports whether s looks like an abbreviation (SSR, RegEx, ASGs)
// and returns it upper-case, without a plural "s".
func abbrev(s string) (string, bool) {
	if len(s) > 1 && s[len(s)-1] == 's' && strings.ToUpper(s[:len(s)-1]) == s[:len(s)-1] {
		s = s[:len(s)-1]
	}
	if len(s) < minAbbrev || len(s) > maxAbbrev || s[0] < 'A' || s[0] > 'Z' {
		return "", false
	}
	upper := 0
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			upper++
		case r < 'a' || r > 'z':
			return "", false
		}
	}
	return strings.ToUpper(s), upper >= 2
}

// spells reports whether abbr is made of leading letters of the words of
// long, in order: every word but a stop word gives at least one letter.
// It needs at least two words (hyphen parts count), otherwise "Kubernetes
// (KUBE)" would just be a prefix.
func spells(abbr string, long []string) bool {
	var parts []string
	for _, w := range long {
		parts = append(parts, strings.Split(strings.ToLower(w), "-")...)
	}
	if len(parts) < 2 || stopWords[parts[0]] {
		return false
	}
	return fits(strings.ToLower(abbr), parts)
}

func fits(ab string, parts []string) bool {
	if len(parts) == 0 {
		return ab == ""
	}
	p := parts[0]
	if stopWords[p] && fits(ab, parts[1:]) {
		return true
	}
	for k := 1; k <= len(p) && k <= len(ab) && p[k-1] == ab[k-1]; k++ {
		if fits(ab[k:], parts[1:]) {
			return true
		}
	}
	return false
}

// Candidate is one suggested concept: a spelled-out name and the
// abbreviations found for it.
type Candidate struct {
	Concept  string
	Synonyms []string
	Count    int      // occurrences
	Docs     []string // docs it was found in, sorted
}

type pairStat struct {
	count int
	docs  map[string]bool
	forms map[string]int // spellings of the name, e.g. singular and plural
}

// Miner collects abbreviation pairs; it is safe for concurrent use.
type Miner struct {
	mu    sync.Mutex
	pairs map[string]map[string]*pairStat // ABBR -> name key -> stat
}

// Add records the pairs found in text of doc.
func (m *Miner) Add(doc, text string) {
	type pair struct{ abbr, long string }
	var found []pair
	FindAbbrevs(text, func(abbr, long string) { found = append(found, pair{abbr, long}) })
	if len(found) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pairs == nil {
		m.pairs = make(map[string]map[string]*pairStat)
	}
	for _, p := range found {
		byName := m.pairs[p.abbr]
		if byName == nil {
			byName = make(map[string]*pairStat)
			m.pairs[p.abbr] = byName
		}
		key := strings.TrimSuffix(p.long, "s") // "access control lists" = "... list"
		st := byName[key]
		if st == nil {
			st = &pairStat{docs: make(map[string]bool), forms: make(map[string]int)}
			byName[key] = st
		}
		st.count++
		st.docs[doc] = true
		st.forms[p.long]++
	}
}

// Candidates returns the pairs found in at least minDocs docs and
// minCount times, whose abbreviation means one thing in most of its
// uses, and which known (may be nil) does not have yet. Abbreviations
// of the same name share a candidate. Most widespread first.
func (m *Miner) Candidates(minDocs, minCount int, known *Thesaurus) []Candidate {
	m.mu.Lock()
	defer m.mu.Unlock()
	byConcept := make(map[string]*Candidate)
	for abbr, byName := range m.pairs {
		total := 0
		var top *pairStat
		for _, st := range byName {
			total += st.count
			if top == nil || len(st.docs) > len(top.docs) || len(st.docs) == len(top.docs) && st.count > top.count {
				top = st
			}
		}
		if len(top.docs) < minDocs || top.count < minCount || float64(top.count) < minShare*float64(total) {
			continue
		}
		concept := conceptForm(top.forms)
		ab := strings.ToLower(abbr)
		if known.Lookup(ab) != nil || known.Lookup(concept) != nil {
			continue
		}
		c := byConcept[concept]
		if c == nil {
			c = &Candidate{Concept: concept}
			byConcept[concept] = c
			// "server-side rendering" is also searched as "server side rendering".
			if spaced := strings.ReplaceAll(concept, "-", " "); spaced != concept {
				c.Synonyms = append(c.Synonyms, spaced)
			}
		}
		c.Synonyms = append([]string{ab}, c.Synonyms...)
		c.Count += top.count
		for d := range top.docs {
			if !contains(c.Docs, d) {
				c.Docs = append(c.Docs, d)
			}
		}
	}
	out := make([]Candidate, 0, len(byConcept))
	for _, c := range byConcept {
		sort.Strings(c.Docs)
		out = append(out, *c)
	}
	sort.Slice(out, func(a, b int) bool {
		if len(out[a].Docs) != len(out[b].Docs) {
			return len(out[a].Docs) > len(out[b].Docs)
		}
		if out[a].Count != out[b].Count {
			return out[a].Count > out[b].Count
		}
		return out[a].Concept < out[b].Concept
	})
	return out
}

// conceptForm picks the spelling of a name to use as the concept: the
// singular when it was seen (a prefix query finds the plural too), else
// the most used one.
func conceptForm(forms map[string]int) string {
	best := ""
	for f, n := range forms {
		if best == "" || n > forms[best] || n == forms[best] && f < best {
			best = f
		}
	}
	if one := strings.TrimSuffix(best, "s"); forms[one] > 0 {
		return one
	}
	return best
}
