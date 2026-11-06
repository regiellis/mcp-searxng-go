package cache

import (
	"testing"
	"time"
)

func TestTTLCacheExpires(t *testing.T) {
	t.Parallel()

	c := New[string](2)
	now := time.Unix(100, 0)
	c.now = func() time.Time { return now }
	c.Set("a", "value", time.Second)

	if got, ok := c.Get("a"); !ok || got != "value" {
		t.Fatalf("expected cached value, got %q, %v", got, ok)
	}

	now = now.Add(2 * time.Second)
	if _, ok := c.Get("a"); ok {
		t.Fatal("expected entry to expire")
	}
}

func TestTTLCacheEvictsOldest(t *testing.T) {
	t.Parallel()

	c := New[int](2)
	now := time.Unix(100, 0)
	c.now = func() time.Time { return now }

	c.Set("a", 1, time.Minute)
	now = now.Add(time.Second)
	c.Set("b", 2, time.Minute)
	now = now.Add(time.Second)
	c.Set("c", 3, time.Minute)

	if _, ok := c.Get("a"); ok {
		t.Fatal("expected oldest key to be evicted")
	}
	if _, ok := c.Get("b"); !ok {
		t.Fatal("expected key b to remain")
	}
	if _, ok := c.Get("c"); !ok {
		t.Fatal("expected key c to remain")
	}
}
