package decoder

import "sync"

// StringCache provides bounded string interning using offset-based indexing.
// Similar to encoding/json/v2's intern.go but uses offsets instead of hashing.
// Thread-safe for concurrent use.
type StringCache struct {
	// Fixed-size cache to prevent unbounded memory growth
	// Using 512 entries for 8KiB total memory footprint (512 * 16 bytes per string)
	cache [512]cacheEntry
	// RWMutex for thread safety - allows concurrent reads, exclusive writes
	mu sync.RWMutex
}

type cacheEntry struct {
	str    string
	offset uint
}

// NewStringCache creates a new bounded string cache.
func NewStringCache() *StringCache {
	return &StringCache{}
}

// InternAt returns a canonical string for the data at the given offset and size.
// Uses the offset modulo cache size as the index, similar to json/v2's approach.
// Thread-safe for concurrent use.
func (sc *StringCache) InternAt(offset, size uint, data []byte) string {
	const (
		minCachedLen = 2   // single byte strings not worth caching
		maxCachedLen = 100 // reasonable upper bound for geographic strings
	)

	// Skip caching for very short or very long strings
	if size < minCachedLen || size > maxCachedLen {
		return string(data[offset : offset+size])
	}

	// Use offset as cache index (modulo cache size)
	i := offset % uint(len(sc.cache))

	// Fast path: check for cache hit with read lock
	sc.mu.RLock()
	entry := sc.cache[i]
	if entry.offset == offset && len(entry.str) == int(size) {
		str := entry.str
		sc.mu.RUnlock()
		return str
	}
	sc.mu.RUnlock()

	// Cache miss - create new string and store with write lock
	str := string(data[offset : offset+size])

	sc.mu.Lock()
	// Double-check in case another goroutine added it while we were waiting
	if sc.cache[i].offset == offset && len(sc.cache[i].str) == int(size) {
		str = sc.cache[i].str
	} else {
		sc.cache[i] = cacheEntry{offset: offset, str: str}
	}
	sc.mu.Unlock()

	return str
}
