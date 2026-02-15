package decoder

import (
	"sync"
)

// cacheEntry represents a cached string with its offset and dedicated mutex.
type cacheEntry struct {
	str    string
	offset uint
	mu     sync.Mutex
}

// stringCache provides bounded string interning with per-entry mutexes for minimal contention.
// This achieves thread safety while avoiding the global lock bottleneck.
type stringCache struct {
	entries [4096]cacheEntry
}

// newStringCache creates a new per-entry mutex-based string cache.
func newStringCache() *stringCache {
	return &stringCache{}
}

// internAt returns a canonical string for the data at the given offset and size.
func (sc *stringCache) internAt(offset, size uint, data []byte) string {
	const (
		minCachedLen = 2   // single byte strings not worth caching
		maxCachedLen = 100 // reasonable upper bound for geographic strings
	)

	// Skip caching for very short or very long strings
	if size < minCachedLen || size > maxCachedLen {
		return string(data[offset : offset+size])
	}

	// Use same cache index calculation as original: offset % cacheSize
	i := offset % uint(len(sc.entries))
	entry := &sc.entries[i]

	entry.mu.Lock()
	if entry.offset == offset && entry.str != "" {
		str := entry.str
		entry.mu.Unlock()
		return str
	}

	// Cache miss - create new string
	str := string(data[offset : offset+size])

	// Store in this slot.
	entry.offset = offset
	entry.str = str
	entry.mu.Unlock()

	return str
}
