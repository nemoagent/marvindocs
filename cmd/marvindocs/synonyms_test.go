package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestReadSynonymTable(t *testing.T) {
	in := "# concept\tsynonyms\tcount\n" +
		"server-side rendering\tssr, server side rendering\t73\t5\treact\n" +
		"\n" +
		"server-side rendering\tssr, серверный рендеринг\n" +
		"write-ahead logging\twal\n"
	got, order, err := readSynonymTable(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"server-side rendering": {"ssr", "server side rendering", "серверный рендеринг"},
		"write-ahead logging":   {"wal"},
	}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(order, []string{"server-side rendering", "write-ahead logging"}) {
		t.Errorf("got %v %v", got, order)
	}
	if _, _, err := readSynonymTable(strings.NewReader("no tab here\n")); err == nil {
		t.Error("want an error for a row without synonyms")
	}
}

func TestSynonymFileName(t *testing.T) {
	cases := map[string]string{
		"server-side rendering":   "server-side-rendering.md",
		"Point-in-Time  Recovery": "point-in-time-recovery.md",
		"node.js":                 "node-js.md",
		"переменная окружения":    "переменная-окружения.md",
	}
	for in, want := range cases {
		if got := synonymFileName(in); got != want {
			t.Errorf("synonymFileName(%q) = %q, want %q", in, got, want)
		}
	}
}
