package index

import "testing"

func TestOpen_AppliesReadPragmas(t *testing.T) {
	idx := substringFixture(t)
	var cache, temp int
	if err := idx.db.QueryRow(`PRAGMA cache_size`).Scan(&cache); err != nil {
		t.Fatal(err)
	}
	if err := idx.db.QueryRow(`PRAGMA temp_store`).Scan(&temp); err != nil {
		t.Fatal(err)
	}
	if cache != -65536 || temp != 2 {
		t.Fatalf("cache_size=%d temp_store=%d, want -65536 and 2", cache, temp)
	}
}
