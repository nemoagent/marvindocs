package synonyms

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestBuiltin(t *testing.T) {
	th, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	if got := th.Lookup("K8s"); !slices.Contains(got, "kubernetes") {
		t.Errorf("Lookup(K8s) = %v, want kubernetes", got)
	}
	if got := th.Lookup("postgresql"); !slices.Contains(got, "pg") {
		t.Errorf("Lookup(postgresql) = %v, want pg", got)
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		// The heading names the concept; several synonyms lines add up.
		"re.md": "# Regular Expression\n\nsynonyms:: regex, regexp\nSynonyms :: re\n",
		// No heading: the file name is the concept.
		"teardown.md": "Some text.\nsynonyms:: cleanup\n",
		"notes.txt":   "synonyms:: ignored\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	th := &Thesaurus{}
	if err := th.LoadDir(dir); err != nil {
		t.Fatal(err)
	}
	if got, want := th.Lookup("regex"), []string{"regular expression", "regexp", "re"}; !slices.Equal(got, want) {
		t.Errorf("Lookup(regex) = %v, want %v", got, want)
	}
	if got, want := th.Lookup("cleanup"), []string{"teardown"}; !slices.Equal(got, want) {
		t.Errorf("Lookup(cleanup) = %v, want %v", got, want)
	}
	if got := th.Lookup("ignored"); got != nil {
		t.Errorf("Lookup(ignored) = %v, want nil", got)
	}
}

func TestMatchLongest(t *testing.T) {
	th := &Thesaurus{}
	th.add([]string{"regular expression", "regex"})
	th.add([]string{"regular", "normal"})

	n, syns := th.Match([]string{"Regular", "Expression", "flags"})
	if n != 2 || !slices.Equal(syns, []string{"regex"}) {
		t.Errorf("Match = %d %v, want 2 [regex]", n, syns)
	}
	n, syns = th.Match([]string{"regular", "flags"})
	if n != 1 || !slices.Equal(syns, []string{"normal"}) {
		t.Errorf("Match = %d %v, want 1 [normal]", n, syns)
	}
	if n, _ := th.Match([]string{"flags"}); n != 0 {
		t.Errorf("Match(flags) = %d, want 0", n)
	}
}

func TestNilThesaurus(t *testing.T) {
	var th *Thesaurus
	if th.Lookup("regex") != nil {
		t.Error("nil Lookup returned synonyms")
	}
	if n, _ := th.Match([]string{"regex"}); n != 0 {
		t.Error("nil Match matched")
	}
}
