package decoder

import (
	"sync"
)

// StringInterner interns strings at MMDB offsets.
type StringInterner interface {
	InternAt(offset, size uint, data []byte) string
}

// cacheEntry represents a cached string with its offset and dedicated mutex.
type cacheEntry struct {
	str    string
	offset uint
	mu     sync.Mutex
}

// stringCache provides bounded string interning with per-entry mutexes for minimal contention.
// This achieves thread safety while avoiding the global lock bottleneck.
type stringCache struct {
	twoLetterStrings [26 * 26]string
	entries          [4096]cacheEntry
	lockEntries      bool
}

// newStringCache creates a new per-entry mutex-based string cache.
func newStringCache() *stringCache {
	sc := &stringCache{}
	sc.lockEntries = true
	initTwoLetterStrings(sc)
	return sc
}

// newStringCacheNoLock creates a cache intended for exclusive goroutine use.
func initTwoLetterStrings(sc *stringCache) {
	for a := byte('a'); a <= 'z'; a++ {
		for b := byte('a'); b <= 'z'; b++ {
			i := int(a-'a')*26 + int(b-'a')
			sc.twoLetterStrings[i] = string([]byte{a, b})
		}
	}
}

// internAt returns a canonical string for the data at the given offset and size.
func (sc *stringCache) InternAt(offset, size uint, data []byte) string {
	const (
		minCachedLen = 2  // single byte strings not worth caching
		maxCachedLen = 32 // favor short, frequently repeated strings
	)

	// Skip caching for very short or very long strings
	if size < minCachedLen || size > maxCachedLen {
		return string(data[offset : offset+size])
	}
	if size == 2 {
		a := data[offset]
		b := data[offset+1]
		if a >= 'a' && a <= 'z' && b >= 'a' && b <= 'z' {
			i := int(a-'a')*26 + int(b-'a')
			return sc.twoLetterStrings[i]
		}
	}

	// Use same cache index calculation as original: offset % cacheSize
	i := offset % uint(len(sc.entries))
	entry := &sc.entries[i]

	if sc.lockEntries {
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

	if entry.offset == offset && entry.str != "" {
		return entry.str
	}

	str := string(data[offset : offset+size])
	entry.offset = offset
	entry.str = str
	return str
}
