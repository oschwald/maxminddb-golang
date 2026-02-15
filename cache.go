package maxminddb

import "github.com/oschwald/maxminddb-golang/v2/internal/decoder"

// Cache interns strings for decoded MMDB values.
type Cache interface {
	InternAt(offset, size uint, data []byte) string
}

// CacheProvider acquires and releases caches for decode operations.
//
// Providers may return a shared thread-safe Cache or a per-decode exclusive
// Cache (e.g., from a pool). Release will always be called after decoding.
type CacheProvider interface {
	Acquire() Cache
	Release(Cache)
}

// CacheOptions configure built-in cache providers.
type CacheOptions struct {
	EntryCount   int
	MinCachedLen uint
	MaxCachedLen uint
}

// DefaultCacheOptions returns default options for built-in cache providers.
func DefaultCacheOptions() CacheOptions {
	o := decoder.DefaultCacheOptions()
	return CacheOptions{
		EntryCount:   o.EntryCount,
		MinCachedLen: o.MinCachedLen,
		MaxCachedLen: o.MaxCachedLen,
	}
}

type builtinCacheProvider struct {
	internal decoder.CacheProvider
}

func (p *builtinCacheProvider) Acquire() Cache {
	return p.internal.Acquire()
}

func (p *builtinCacheProvider) Release(c Cache) {
	if c == nil {
		return
	}
	if interner, ok := c.(decoder.StringInterner); ok {
		p.internal.Release(interner)
		return
	}
	p.internal.Release(cacheAdapter{Cache: c})
}

func (p *builtinCacheProvider) internalProvider() decoder.CacheProvider {
	return p.internal
}

// NewSharedCacheProvider creates a provider using one shared lock-based cache.
func NewSharedCacheProvider(opts CacheOptions) CacheProvider {
	return &builtinCacheProvider{
		internal: decoder.NewSharedCacheProvider(decoder.CacheOptions{
			EntryCount:   opts.EntryCount,
			MinCachedLen: opts.MinCachedLen,
			MaxCachedLen: opts.MaxCachedLen,
		}),
	}
}

// NewPooledCacheProvider creates a provider using pooled exclusive caches.
func NewPooledCacheProvider(opts CacheOptions) CacheProvider {
	return &builtinCacheProvider{
		internal: decoder.NewPooledCacheProvider(decoder.CacheOptions{
			EntryCount:   opts.EntryCount,
			MinCachedLen: opts.MinCachedLen,
			MaxCachedLen: opts.MaxCachedLen,
		}),
	}
}

type cacheAdapter struct {
	Cache
}

type userCacheProviderAdapter struct {
	provider CacheProvider
}

func (a *userCacheProviderAdapter) Acquire() decoder.StringInterner {
	c := a.provider.Acquire()
	if c == nil {
		return nil
	}
	if interner, ok := c.(decoder.StringInterner); ok {
		return interner
	}
	return cacheAdapter{Cache: c}
}

func (a *userCacheProviderAdapter) Release(interner decoder.StringInterner) {
	if interner == nil {
		a.provider.Release(nil)
		return
	}
	if c, ok := interner.(Cache); ok {
		a.provider.Release(c)
		return
	}
	// Should not happen, but preserve behavior by discarding unknown wrapper.
	a.provider.Release(nil)
}

type internalCacheProvider interface {
	internalProvider() decoder.CacheProvider
}

func toInternalCacheProvider(provider CacheProvider) decoder.CacheProvider {
	if provider == nil {
		return nil
	}
	if p, ok := provider.(internalCacheProvider); ok {
		return p.internalProvider()
	}
	return &userCacheProviderAdapter{provider: provider}
}
