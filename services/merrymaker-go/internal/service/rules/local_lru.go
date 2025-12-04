package rules

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"
)

// LocalLRU is a small in-memory LRU cache with per-entry TTL.
// It stores byte slices and is intended as the first tier in the rules caching stack.
// Concurrency: methods are safe for concurrent use.
type LocalLRU struct {
	mu     sync.Mutex
	cap    int
	ll     *list.List               // front = most-recently used
	items  map[string]*list.Element // key -> element
	now    func() time.Time         // injectable clock for tests
	hits   atomic.Uint64
	misses atomic.Uint64
	evicts atomic.Uint64
}

type lruEntry struct {
	key    string
	value  []byte
	expiry time.Time // zero means no expiry
}

// LocalLRUConfig groups constructor options (<=3 params rule).
type LocalLRUConfig struct {
	Capacity int
	Now      func() time.Time
}

// DefaultLocalLRUConfig returns sensible defaults.
func DefaultLocalLRUConfig() LocalLRUConfig {
	return LocalLRUConfig{Capacity: 1024, Now: time.Now}
}

// NewLocalLRU creates a new LocalLRU with the given config.
func NewLocalLRU(cfg LocalLRUConfig) *LocalLRU {
	capacity := cfg.Capacity
	if capacity <= 0 {
		capacity = 1024
	}
	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	return &LocalLRU{
		cap:   capacity,
		ll:    list.New(),
		items: make(map[string]*list.Element, capacity),
		now:   nowFn,
	}
}

// Get returns the value for key if present and not expired.
func (c *LocalLRU) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, found := c.items[key]; found {
		ent, entryOK := el.Value.(*lruEntry)
		if !entryOK {
			c.removeElement(el)
			c.misses.Add(1)
			return nil, false
		}
		if c.isExpired(ent) {
			c.removeElement(el)
			c.misses.Add(1)
			return nil, false
		}
		c.ll.MoveToFront(el)
		c.hits.Add(1)
		return ent.value, true
	}
	c.misses.Add(1)
	return nil, false
}

// Exists returns true if key is present and not expired.
func (c *LocalLRU) Exists(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, found := c.items[key]; found {
		ent, entryOK := el.Value.(*lruEntry)
		if !entryOK {
			c.removeElement(el)
			return false
		}
		if c.isExpired(ent) {
			c.removeElement(el)
			return false
		}
		c.ll.MoveToFront(el)
		return true
	}
	return false
}

// Set inserts or updates a value with TTL.
// ttl <= 0 means no expiration.
func (c *LocalLRU) Set(key string, value []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var exp time.Time
	if ttl > 0 {
		exp = c.now().Add(ttl)
	}

	if el, found := c.items[key]; found {
		if ent, entryOK := el.Value.(*lruEntry); entryOK {
			ent.value = value
			ent.expiry = exp
			c.ll.MoveToFront(el)
			return
		}
		// If the element has an unexpected value type, remove and recreate it.
		c.removeElement(el)
	}

	el := c.ll.PushFront(&lruEntry{key: key, value: value, expiry: exp})
	c.items[key] = el
	c.evictIfNeeded()
}

// Delete removes a key from the cache.
func (c *LocalLRU) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		c.removeElement(el)
		return true
	}
	return false
}

// Len returns the current number of items in the cache.
func (c *LocalLRU) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

// LocalLRUStats are simple counters for observability.
type LocalLRUStats struct {
	Hits, Misses, Evictions uint64
	Size, Capacity          int
}

// Stats returns a snapshot of counters and sizes.
func (c *LocalLRU) Stats() LocalLRUStats {
	return LocalLRUStats{
		Hits:      c.hits.Load(),
		Misses:    c.misses.Load(),
		Evictions: c.evicts.Load(),
		Size:      c.Len(),
		Capacity:  c.cap,
	}
}

// Helpers (caller must hold c.mu where noted).
func (c *LocalLRU) isExpired(e *lruEntry) bool {
	if e.expiry.IsZero() {
		return false
	}
	return c.now().After(e.expiry)
}

func (c *LocalLRU) removeElement(el *list.Element) {
	c.ll.Remove(el)
	if ent, ok := el.Value.(*lruEntry); ok {
		delete(c.items, ent.key)
		return
	}
	for k, v := range c.items {
		if v == el {
			delete(c.items, k)
			break
		}
	}
}

func (c *LocalLRU) evictIfNeeded() {
	for c.ll.Len() > c.cap {
		el := c.ll.Back()
		if el == nil {
			return
		}
		c.removeElement(el)
		c.evicts.Add(1)
	}
}
