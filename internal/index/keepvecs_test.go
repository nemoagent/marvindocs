package index

import (
	"reflect"
	"testing"
)

func TestVectorDocs(t *testing.T) {
	in := []string{
		"python~3.11", "python~3.12", "python~3.13", "python~3.14", "python~2.7",
		"node", "node~20_lts", "node~4_lts",
		"gcc~12", "gcc~13", "gcc~14", "gcc~11", "gcc~12_cpp", "gcc~13_cpp", "gcc~14_cpp", "gcc~11_cpp",
		"haxe", "haxe~cpp", "haxe~java", "haxe~python", "haxe~lua",
		"django~4.2", "django~5.0", "django~5.1", "django~5.2", "django~6.0",
		"react",
	}
	want := []string{
		"django~4.2", "django~5.1", "django~5.2", "django~6.0",
		"gcc~12", "gcc~12_cpp", "gcc~13", "gcc~13_cpp", "gcc~14", "gcc~14_cpp",
		"haxe", "haxe~cpp", "haxe~java", "haxe~lua", "haxe~python",
		"node", "node~20_lts", "node~4_lts",
		"python~3.12", "python~3.13", "python~3.14",
		"react",
	}
	if got := vectorDocs(in); !reflect.DeepEqual(got, want) {
		t.Errorf("vectorDocs:\n got %v\nwant %v", got, want)
	}
}

func TestNewestDocs(t *testing.T) {
	in := []string{
		"python~3.13", "python~3.14", "node", "node~20_lts", "node~24_lts",
		"gcc~13", "gcc~14", "gcc~13_cpp", "gcc~14_cpp", "haxe~cpp", "openjdk~21", "openjdk~25", "react",
	}
	lts := map[string]bool{"node~24_lts": true, "openjdk~21": true}
	want := []string{
		"gcc~14", "gcc~14_cpp", "haxe~cpp", "node", "node~24_lts",
		"openjdk~21", "openjdk~25", "python~3.14", "react",
	}
	if got := NewestDocs(in, lts); !reflect.DeepEqual(got, want) {
		t.Errorf("NewestDocs:\n got %v\nwant %v", got, want)
	}
}
