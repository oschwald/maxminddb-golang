package decoder

import "sync"

// CacheProvider provides interners for decode operations.
type CacheProvider interface {
	Acquire() StringInterner
	Release(StringInterner)
}

// CacheOptions configure built-in cache providers.
type CacheOptions struct {
	EntryCount   int
	MinCachedLen uint
	MaxCachedLen uint
}

// DefaultCacheOptions returns the built-in cache defaults.
func DefaultCacheOptions() CacheOptions {
	return CacheOptions{
		EntryCount:   4096,
		MinCachedLen: 2,
		MaxCachedLen: 32,
	}
}

func (o CacheOptions) normalized() CacheOptions {
	def := DefaultCacheOptions()
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

type sharedCacheProvider struct {
	cache StringInterner
}

func (p *sharedCacheProvider) Acquire() StringInterner {
	return p.cache
}

func (*sharedCacheProvider) Release(StringInterner) {}

// NewSharedCacheProvider creates a provider that returns one shared lock-based
// cache instance.
func NewSharedCacheProvider(opts CacheOptions) CacheProvider {
	opts = opts.normalized()
	return &sharedCacheProvider{
		cache: newConfigurableStringCache(opts, true),
	}
}

type pooledCacheProvider struct {
	pool *sync.Pool
}

func (p *pooledCacheProvider) Acquire() StringInterner {
	v := p.pool.Get()
	cache, _ := v.(StringInterner)
	if cache == nil {
		return newConfigurableStringCache(DefaultCacheOptions(), false)
	}
	return cache
}

func (p *pooledCacheProvider) Release(c StringInterner) {
	if c == nil {
		return
	}
	p.pool.Put(c)
}

// NewPooledCacheProvider creates a provider that returns an exclusive no-lock
// cache from a pool per decode call.
func NewPooledCacheProvider(opts CacheOptions) CacheProvider {
	opts = opts.normalized()
	return &pooledCacheProvider{
		pool: &sync.Pool{
			New: func() any {
				return newConfigurableStringCache(opts, false)
			},
		},
	}
}

type noCacheInterner struct{}

func (noCacheInterner) InternAt(offset, size uint, data []byte) string {
	return string(data[offset : offset+size])
}

type noCacheProvider struct {
	cache noCacheInterner
}

func (p *noCacheProvider) Acquire() StringInterner {
	return p.cache
}

func (*noCacheProvider) Release(StringInterner) {}

// NewNoCacheProvider creates a provider that disables interning.
func NewNoCacheProvider() CacheProvider {
	return &noCacheProvider{}
}

type configurableStringCache struct {
	twoLetterStrings [26 * 26]string
	entries          []cacheEntry
	entryMask        uint
	minCachedLen     uint
	maxCachedLen     uint
	lockEntries      bool
}

func newConfigurableStringCache(opts CacheOptions, lockEntries bool) *configurableStringCache {
	opts = opts.normalized()
	sc := &configurableStringCache{
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

func (sc *configurableStringCache) InternAt(offset, size uint, data []byte) string {
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
