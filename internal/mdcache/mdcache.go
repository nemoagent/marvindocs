// Package mdcache is a small in-memory LRU for HTML→Markdown conversions.
//
// The HTML-to-markdown converter is the heaviest single operation in
// the serve path: it parses an HTML fragment and walks the DOM. For
// hot pages (popular APIs hit by MCP agents many times in a row)
// caching the converted Markdown removes that cost entirely.
//
// The cache is keyed by (slug, path, release) so a re-indexed doc
// (different release) does not collide with stale entries.
package mdcache

import (
	"container/list"
	"sync"
)

// Cache is a fixed-size LRU keyed by string. Values are stored by
// value (strings) — there's no need for atomics or weak refs because
// strings are immutable in Go.
type Cache struct {
	mu       sync.Mutex
	capacity int
	ll       *list.List
	idx      map[string]*list.Element
}

type entry struct {
	key string
	val string
}

// New creates a Cache that holds up to capacity entries. capacity <= 0
// returns a no-op cache (Get always misses, Add does nothing).
func New(capacity int) *Cache {
	return &Cache{
		capacity: capacity,
		ll:       list.New(),
		idx:      make(map[string]*list.Element),
	}
}

// Key builds the cache key from the parts that identify a converted
// page. Release is included so cache entries are invalidated on
// re-index.
func Key(slug, path, release string) string {
	return slug + "\x00" + release + "\x00" + path
}

// Get looks up val by key. ok is false on miss or when the cache is
// disabled.
func (c *Cache) Get(key string) (val string, ok bool) {
	if c == nil || c.capacity <= 0 {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, hit := c.idx[key]; hit {
		c.ll.MoveToFront(el)
		return el.Value.(*entry).val, true
	}
	return "", false
}

// Add inserts or replaces an entry, evicting the least-recently-used
// one if the cache is full.
func (c *Cache) Add(key, val string) {
	if c == nil || c.capacity <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, hit := c.idx[key]; hit {
		el.Value.(*entry).val = val
		c.ll.MoveToFront(el)
		return
	}
	el := c.ll.PushFront(&entry{key: key, val: val})
	c.idx[key] = el
	if c.ll.Len() > c.capacity {
		old := c.ll.Back()
		if old != nil {
			c.ll.Remove(old)
			delete(c.idx, old.Value.(*entry).key)
		}
	}
}

// Len returns the current number of cached entries.
func (c *Cache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}
