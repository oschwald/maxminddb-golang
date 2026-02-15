// Package cache provides pluggable string interning caches for maxminddb
// decoding.
package cache

import "sync"

// Cache interns strings at MMDB offsets.
type Cache interface {
	InternAt(offset, size uint, data []byte) string
}

// Provider acquires and releases caches for decode operations.
//
// Providers may return a shared thread-safe Cache or a per-decode exclusive
// Cache (e.g., from a pool). Release is called after decoding.
type Provider interface {
	Acquire() Cache
	Release(Cache)
}

// Options configure built-in cache providers.
type Options struct {
	EntryCount   int
	MinCachedLen uint
	MaxCachedLen uint
}

// DefaultOptions returns the built-in cache defaults.
func DefaultOptions() Options {
	return Options{
		EntryCount:   4096,
		MinCachedLen: 2,
		MaxCachedLen: 32,
	}
}

func (o Options) normalized() Options {
	def := DefaultOptions()
	out := o
	if out.EntryCount <= 0 {
		out.EntryCount = def.EntryCount
	}
	if out.MinCachedLen == 0 {
		out.MinCachedLen = def.MinCachedLen
	}
	if out.MaxCachedLen == 0 {
		out.MaxCachedLen = def.MaxCachedLen
	}
	if out.MaxCachedLen < out.MinCachedLen {
		out.MaxCachedLen = out.MinCachedLen
	}
	return out
}

type cacheEntry struct {
	str    string
	offset uint
	mu     sync.Mutex
}

type configurableCache struct {
	twoLetterStrings [26 * 26]string
	entries          []cacheEntry
	entryMask        uint
	minCachedLen     uint
	maxCachedLen     uint
	lockEntries      bool
}

func newConfigurableCache(opts Options, lockEntries bool) *configurableCache {
	opts = opts.normalized()
	sc := &configurableCache{
		lockEntries:  lockEntries,
		minCachedLen: opts.MinCachedLen,
		maxCachedLen: opts.MaxCachedLen,
		entries:      make([]cacheEntry, opts.EntryCount),
	}
	if opts.EntryCount&(opts.EntryCount-1) == 0 {
		sc.entryMask = uint(opts.EntryCount - 1)
	}
	for a := byte('a'); a <= 'z'; a++ {
		for b := byte('a'); b <= 'z'; b++ {
			i := int(a-'a')*26 + int(b-'a')
			sc.twoLetterStrings[i] = string([]byte{a, b})
		}
	}
	return sc
}

func (sc *configurableCache) InternAt(offset, size uint, data []byte) string {
	if size < sc.minCachedLen || size > sc.maxCachedLen {
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

	var i uint
	if sc.entryMask != 0 {
		i = offset & sc.entryMask
	} else {
		i = offset % uint(len(sc.entries))
	}
	entry := &sc.entries[i]

	if sc.lockEntries {
		entry.mu.Lock()
		if entry.offset == offset && entry.str != "" {
			str := entry.str
			entry.mu.Unlock()
			return str
		}
		str := string(data[offset : offset+size])
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

type sharedProvider struct {
	cache Cache
}

func (p *sharedProvider) Acquire() Cache {
	return p.cache
}

func (*sharedProvider) Release(Cache) {}

// NewSharedProvider creates a provider that returns one shared lock-based
// cache instance.
func NewSharedProvider(opts Options) Provider {
	opts = opts.normalized()
	return &sharedProvider{
		cache: newConfigurableCache(opts, true),
	}
}

type pooledProvider struct {
	pool *sync.Pool
}

func (p *pooledProvider) Acquire() Cache {
	v := p.pool.Get()
	c, _ := v.(Cache)
	if c == nil {
		return newConfigurableCache(DefaultOptions(), false)
	}
	return c
}

func (p *pooledProvider) Release(c Cache) {
	if c == nil {
		return
	}
	p.pool.Put(c)
}

// NewPooledProvider creates a provider that returns an exclusive no-lock cache
// from a pool per decode call.
func NewPooledProvider(opts Options) Provider {
	opts = opts.normalized()
	return &pooledProvider{
		pool: &sync.Pool{
			New: func() any {
				return newConfigurableCache(opts, false)
			},
		},
	}
}

type noCache struct{}

func (noCache) InternAt(offset, size uint, data []byte) string {
	return string(data[offset : offset+size])
}

type noCacheProvider struct {
	cache noCache
}

func (p *noCacheProvider) Acquire() Cache {
	return p.cache
}

func (*noCacheProvider) Release(Cache) {}

// NewNoCacheProvider creates a provider that disables interning.
func NewNoCacheProvider() Provider {
	return &noCacheProvider{}
}
