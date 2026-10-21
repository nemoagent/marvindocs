package synonyms

import (
	"reflect"
	"testing"
)

func TestFindAbbrevs(t *testing.T) {
	cases := []struct {
		text string
		want []string // "ABBR=long"
	}{
		{"Server-Side Rendering (SSR)", []string{"SSR=server-side rendering"}},
		{"Migrating from Create React App (CRA)", []string{"CRA=create react app"}},
		{"see write-ahead logging (WAL) and Point-in-Time Recovery (PITR).", []string{"WAL=write-ahead logging", "PITR=point-in-time recovery"}},
		{"Internet of Things (IoT)", []string{"IOT=internet of things"}},
		{"Auto Scaling Groups (ASGs)", []string{"ASG=auto scaling groups"}},
		{"CSP (Content Security Policy) headers", []string{"CSP=content security policy"}},
		{"Regular Expression (RegEx)", []string{"REGEX=regular expression"}},
		{"Manage bridge domains (BD)", nil},                 // too short
		{"Kubernetes (KUBE)", nil},                          // one word: just a prefix
		{"call foo (bar) now", nil},                         // not an abbreviation
		{"the API (Application Programming Interface", nil}, // unclosed
		{"Something else (SSR)", nil},                       // letters do not match
		{"Hello, Server (SR)", nil},                         // too short
	}
	for _, c := range cases {
		var got []string
		FindAbbrevs(c.text, func(abbr, long string) { got = append(got, abbr+"="+long) })
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("FindAbbrevs(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestMinerCandidates(t *testing.T) {
	var m Miner
	for _, doc := range []string{"react", "vue", "angular"} {
		m.Add(doc, "Server-Side Rendering (SSR) and SSR (server-side rendering)")
	}
	m.Add("x", "Access Control Lists (ACLs), ACL (access control list)")
	m.Add("y", "Access Control Lists (ACLs)")
	m.Add("react", "Single Page Application (SPA)") // one doc only
	m.Add("a", "Bridge Domain Name (BDN)")          // ambiguous: three meanings
	m.Add("b", "Big Data Node (BDN)")
	m.Add("c", "Basic Disk Number (BDN)")
	m.Add("a", "Regular Expression Group (REG)") // known concept below
	m.Add("b", "Regular Expression Group (REG)")
	m.Add("c", "Regular Expression Group (REG)")

	known := &Thesaurus{}
	known.add([]string{"regular expression group", "rxg"})
	got := m.Candidates(2, 3, known)
	want := []Candidate{{
		Concept:  "server-side rendering",
		Synonyms: []string{"ssr", "server side rendering"},
		Count:    6,
		Docs:     []string{"angular", "react", "vue"},
	}, {
		Concept:  "access control list",
		Synonyms: []string{"acl"},
		Count:    3,
		Docs:     []string{"x", "y"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Candidates = %+v, want %+v", got, want)
	}
}
