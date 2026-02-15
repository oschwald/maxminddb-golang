package maxminddb

import (
	"github.com/oschwald/maxminddb-golang/v2/cache"
	"github.com/oschwald/maxminddb-golang/v2/internal/decoder"
)

type cacheAdapter struct {
	cache.Cache
}

type userCacheProviderAdapter struct {
	provider cache.Provider
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
	if c, ok := interner.(cache.Cache); ok {
		a.provider.Release(c)
		return
	}
	// Should not happen, but preserve behavior by discarding unknown wrapper.
	a.provider.Release(nil)
}

func toInternalCacheProvider(provider cache.Provider) decoder.CacheProvider {
	if provider == nil {
		return nil
	}
	return &userCacheProviderAdapter{provider: provider}
}
