package decoder

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStringCacheLateMissDoesNotDuplicateResident(t *testing.T) {
	cache := newStringCache()
	value := []byte("hello")
	cache.internAt(0, value)
	cache.internAt(0, value)
	resident := cachedEntry(cache, 0)
	require.NotNil(t, resident)

	// Both readers missed before another reader admitted this offset.
	cache.initTable().miss(cache.initTable().bucket(0), 0, value)
	cache.initTable().miss(cache.initTable().bucket(0), 0, value)
	count := 0
	for i := range cache.initTable().bucket(0).entries {
		if entry := cache.initTable().bucket(0).entries[i].Load(); entry != nil &&
			entry.offset == 0 {
			count++
		}
	}
	require.Equal(t, 1, count)
	require.Same(t, resident, cachedEntry(cache, 0))
	require.False(
		t,
		cache.initTable().searching.Load(),
		"duplicate admission must not enable scanning",
	)
}

// This test covers the duplicate scan before acquiring the writer bit. The
// protected scan's ABA interleaving has no deterministic regression test:
// forcing a publication between the scans would require a runtime test hook.
func TestStringCacheLateDuplicateDoesNotAllocate(t *testing.T) {
	for _, test := range []struct {
		name   string
		writer uint64
	}{
		{name: "idle_writer"},
		{name: "busy_writer", writer: stringCacheWriting},
	} {
		t.Run(test.name, func(t *testing.T) {
			table, bucket, value := newLateDuplicateCache()
			referenced := uint64(1) << stringCacheReferencedShift
			control := bucket.control.Load() &^ referenced
			var got string
			allocs := testing.AllocsPerRun(100, func() {
				bucket.control.Store(control | test.writer)
				// A prior miss permits admission, but a competing reader has published.
				table.recentMiss(0).Store(uint64(stringCacheAdmissionValue(0)))
				got = table.miss(bucket, 0, value)
			})
			require.Equal(t, "hello", got)
			require.Zero(t, allocs, "a late duplicate must reuse the published string")
			require.Equal(t, test.writer, bucket.control.Load()&stringCacheWriting,
				"duplicate lookup changed writer ownership")
			require.NotZero(t, bucket.control.Load()&referenced,
				"duplicate lookup did not mark the resident referenced")
			require.Zero(
				t,
				table.recentMiss(0).Load(),
				"duplicate lookup did not consume the miss token",
			)
			if test.writer != 0 {
				bucket.releaseWriter(control)
			}
		})
	}
}

func TestStringCacheWriterPreservesReaderReference(t *testing.T) {
	for _, publish := range []bool{false, true} {
		name := "aborted"
		if publish {
			name = "published"
		}
		t.Run(name, func(t *testing.T) {
			cache := newStringCache()
			for _, offset := range []uint{0, 3} {
				internTestValue(cache, offset)
				internTestValue(cache, offset)
			}
			cache.initTable().searching.Store(true)
			bucket := cache.initTable().bucket(0)
			control := bucket.control.Load() &^ stringCacheReferencedBits
			bucket.control.Store(control | stringCacheWriting)

			// A real lookup references a resident after the writer's snapshot.
			internTestValue(cache, 3)
			referenced := uint64(1) << (stringCacheReferencedShift + stringCacheHomeSlot(3))
			require.NotZero(t, bucket.control.Load()&referenced)
			updated := control
			if publish {
				// Publish another slot while preserving the referenced resident.
				const slot = 2
				offset := uint(6)
				bucket.entries[slot].Store(&cacheEntry{offset: offset, str: "new value"})
				updated |= stringCacheControlByte(offset) << (8 * slot)
				updated |= uint64(1) << (stringCacheReferencedShift + slot)
			}
			bucket.releaseWriter(updated)
			require.NotZero(t, bucket.control.Load()&referenced, "writer discarded a hit")
			require.Zero(t, bucket.control.Load()&stringCacheWriting)
			require.Equal(t, updated&^stringCacheReferencedBits,
				bucket.control.Load()&^stringCacheReferencedBits)
		})
	}
}

func TestStringCacheAllHomeAgingProtectsActiveEntries(t *testing.T) {
	cache := newStringCache()
	// Distinct table passes put each resident at home in the same bucket.
	residents := []uint{
		0, (1 << stringCachePassShift) + 3,
		(2 << stringCachePassShift) + 5, (3 << stringCachePassShift) + 7,
		(4 << stringCachePassShift) + 10, (5 << stringCachePassShift) + 12,
		(6 << stringCachePassShift) + 14,
	}
	for _, offset := range residents {
		internTestValue(cache, offset)
		internTestValue(cache, offset)
		require.NotNil(t, cachedEntry(cache, offset))
	}
	require.False(t, cache.initTable().searching.Load())
	newcomer := uint(7 << stringCachePassShift)
	internTestValue(cache, newcomer)
	internTestValue(cache, newcomer)
	require.True(t, cache.initTable().searching.Load(), "aging must enable reference tracking")
	require.Nil(t, cachedEntry(cache, newcomer))
	for _, offset := range residents[1:] {
		internTestValue(cache, offset)
	}
	internTestValue(cache, newcomer)
	require.NotNil(t, cachedEntry(cache, newcomer))
	require.Nil(t, cachedEntry(cache, residents[0]))
	for _, offset := range residents[1:] {
		require.NotNil(t, cachedEntry(cache, offset), "active resident was evicted")
	}
}

func TestStringCacheAdmissionTokenWrap(t *testing.T) {
	cache := newStringCache()
	const offset = uint(0xffffffff)
	value := []byte("boundary value")
	// A zero admission token cannot distinguish an empty history slot.
	// Bypass caching at this rare boundary instead of admitting a one-off.
	for range 3 {
		require.Equal(t, string(value), cache.internAt(offset, value))
		require.Nil(t, cachedEntry(cache, offset))
	}
}

func newLateDuplicateCache() (*stringCacheTable, *cacheBucket, []byte) {
	cache := newStringCache()
	value := []byte("hello")
	for range 2 {
		cache.internAt(0, value)
	}
	table := cache.initTable()
	return table, table.bucket(0), value
}
