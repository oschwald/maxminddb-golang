package decoder

// CacheProvider provides interners for decode operations.
type CacheProvider interface {
	Acquire() StringInterner
	Release(StringInterner)
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
