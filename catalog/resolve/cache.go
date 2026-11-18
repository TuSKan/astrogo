package resolve

import (
	"sync"
)

// Cache provides a thread-safe caching system for query results.
type Cache interface {
	Get(key string) (SeqIterator[Target], bool)
	Set(key string, items []Target) error
	Close() error
}

// MapCache is an in-memory [Cache] that stores Target slices directly. It is
// safe for concurrent use.
type MapCache struct {
	items map[string][]Target
	mu    sync.RWMutex
}

// NewMapCache returns a ready-to-use in-memory MapCache.
func NewMapCache() *MapCache {
	return &MapCache{
		items: make(map[string][]Target),
	}
}

// Get retrieves cached targets for the given query key, returning a streaming
// iterator and true if the key was found.
func (c *MapCache) Get(key string) (SeqIterator[Target], bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	items, ok := c.items[key]
	if !ok {
		return nil, false
	}

	// Return a snapshot copy to prevent mutation of cached data.
	snapshot := make([]Target, len(items))
	copy(snapshot, items)

	return func(yield func(Target, error) bool) {
		for _, t := range snapshot {
			if !yield(t, nil) {
				return
			}
		}
	}, true
}

// Set stores a slice of targets under the given query key.
func (c *MapCache) Set(key string, items []Target) error {
	// Store a defensive copy to prevent caller mutation.
	stored := make([]Target, len(items))
	copy(stored, items)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = stored

	return nil
}

// Close clears all cached entries.
func (c *MapCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string][]Target)

	return nil
}
