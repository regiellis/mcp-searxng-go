package cache

import (
	"sync"
	"time"
)

type entry[T any] struct {
	value     T
	expiresAt time.Time
	createdAt time.Time
}

// TTLCache is a small thread-safe in-memory TTL cache.
type TTLCache[T any] struct {
	mu         sync.RWMutex
	items      map[string]entry[T]
	maxEntries int
	now        func() time.Time
}

// New creates a TTL cache with a bounded number of entries.
func New[T any](maxEntries int) *TTLCache[T] {
	if maxEntries <= 0 {
		maxEntries = 256
	}
	return &TTLCache[T]{
		items:      make(map[string]entry[T]),
		maxEntries: maxEntries,
		now:        time.Now,
	}
}

// Get returns a cached value if it exists and is not expired.
func (c *TTLCache[T]) Get(key string) (T, bool) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()

	if !ok {
		var zero T
		return zero, false
	}
	if c.now().After(item.expiresAt) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		var zero T
		return zero, false
	}
	return item.value, true
}

// Set stores a value until the given TTL expires.
func (c *TTLCache[T]) Set(key string, value T, ttl time.Duration) {
	if ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.evictExpiredLocked()
	if len(c.items) >= c.maxEntries {
		c.evictOldestLocked()
	}
	now := c.now()
	c.items[key] = entry[T]{
		value:     value,
		createdAt: now,
		expiresAt: now.Add(ttl),
	}
}

func (c *TTLCache[T]) evictExpiredLocked() {
	now := c.now()
	for key, item := range c.items {
		if now.After(item.expiresAt) {
			delete(c.items, key)
		}
	}
}

func (c *TTLCache[T]) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	first := true
	for key, item := range c.items {
		if first || item.createdAt.Before(oldest) {
			oldestKey = key
			oldest = item.createdAt
			first = false
		}
	}
	if !first {
		delete(c.items, oldestKey)
	}
}
