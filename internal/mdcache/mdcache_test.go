package mdcache

import "testing"

func TestCacheBasic(t *testing.T) {
	c := New(2)

	if _, ok := c.Get("a"); ok {
		t.Fatal("empty cache should miss")
	}
	c.Add("a", "1")
	c.Add("b", "2")
	if v, ok := c.Get("a"); !ok || v != "1" {
		t.Fatalf("expected a=1, got %q ok=%v", v, ok)
	}
	// Adding a third entry evicts the LRU. After Get("a"), b is LRU.
	c.Add("c", "3")
	if _, ok := c.Get("b"); ok {
		t.Fatal("b should have been evicted")
	}
	if v, ok := c.Get("a"); !ok || v != "1" {
		t.Fatalf("a should still be present, got %q ok=%v", v, ok)
	}
	if v, ok := c.Get("c"); !ok || v != "3" {
		t.Fatalf("expected c=3, got %q ok=%v", v, ok)
	}
}

func TestCacheDisabled(t *testing.T) {
	c := New(0)
	c.Add("a", "1")
	if _, ok := c.Get("a"); ok {
		t.Fatal("disabled cache should not retain entries")
	}
	if c.Len() != 0 {
		t.Fatalf("expected len=0, got %d", c.Len())
	}
}

func TestCacheReplace(t *testing.T) {
	c := New(2)
	c.Add("a", "1")
	c.Add("a", "2")
	if v, _ := c.Get("a"); v != "2" {
		t.Fatalf("expected updated value, got %q", v)
	}
	if c.Len() != 1 {
		t.Fatalf("expected len=1 after replace, got %d", c.Len())
	}
}
