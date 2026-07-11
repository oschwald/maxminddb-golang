package decoder

import (
	"sync"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

var benchmarkStringCacheSink string

func TestStringCacheVariousOffsets(t *testing.T) {
	cache := newStringCache()
	data := []byte("abcdefghijklmnopqrstuvwxyz")

	testCases := []struct {
		offset   uint
		size     uint
		expected string
	}{
		{0, 3, "abc"},
		{5, 3, "fgh"},
		{10, 5, "klmno"},
		{23, 3, "xyz"},
	}

	for _, tc := range testCases {
		// Repeat 3x: first miss records, second miss admits, third hits.
		for range 3 {
			got := cache.internAt(tc.offset, tc.size, data)
			require.Equal(t, tc.expected, got)
		}
	}
}

// TestStringCacheTwoMissAdmission verifies the admission policy: the first
// miss at a given offset records the offset in recentMisses but does not
// store an entry; the second miss at the same offset admits the entry; the
// third call hits the cache and returns the admitted string verbatim.
func TestStringCacheTwoMissAdmission(t *testing.T) {
	cache := newStringCache()
	data := []byte("hello world, this is test data")

	str1 := cache.internAt(0, 5, data)
	require.Equal(t, "hello", str1)
	require.Nil(t, cache.entries[0].Load(),
		"first miss must not admit (one-off offsets should not allocate cache slots)")

	str2 := cache.internAt(0, 5, data)
	require.Equal(t, "hello", str2)
	entry := cache.entries[0].Load()
	require.NotNil(t, entry, "second miss at same offset must admit")
	require.Equal(t, uint(0), entry.offset)
	require.Equal(t, "hello", entry.str)

	str3 := cache.internAt(0, 5, data)
	require.Equal(t, "hello", str3)
	require.Equal(t,
		//nolint:gosec // test only
		unsafe.StringData(entry.str), unsafe.StringData(str3),
		"cache hit must return the admitted string's backing data, not a fresh allocation")
}

// TestStringCacheNoAdmissionOnSlotCollision verifies that two offsets mapping
// to the same slot alternately miss without admitting. The Swap rule encodes
// "saw the same offset twice in a row" — alternation never matches, so the
// slot stays empty and we avoid thrashing the cache with one-off collisions.
func TestStringCacheNoAdmissionOnSlotCollision(t *testing.T) {
	cache := newStringCache()
	const slotCount = uint(4096)
	data := make([]byte, slotCount+5)
	for i := range data {
		data[i] = 'a' + byte(i%26)
	}

	const offsetA, offsetB uint = 0, slotCount
	require.Equal(t, offsetA%slotCount, offsetB%slotCount,
		"test setup requires colliding slots")

	for range 4 {
		_ = cache.internAt(offsetA, 5, data)
		require.Nil(t, cache.entries[offsetA%slotCount].Load(),
			"alternating offsets must not admit at offsetA miss")
		_ = cache.internAt(offsetB, 5, data)
		require.Nil(t, cache.entries[offsetB%slotCount].Load(),
			"alternating offsets must not admit at offsetB miss")
	}
}

// TestStringCacheConcurrent stresses the lock-free cache with multiple
// goroutines hammering shared offsets. With -race this catches torn reads
// and admission-race bugs; without -race it still verifies that returned
// strings always carry the correct content under contention and that hot
// offsets are eventually admitted.
func TestStringCacheConcurrent(t *testing.T) {
	cache := newStringCache()
	data := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJ")

	offsets := []uint{0, 5, 11, 17, 23}
	const size = uint(5)
	expected := make([]string, len(offsets))
	for i, off := range offsets {
		expected[i] = string(data[off : off+size])
	}

	const goroutines = 16
	const iterations = 5000
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for iter := range iterations {
				idx := iter % len(offsets)
				got := cache.internAt(offsets[idx], size, data)
				if got != expected[idx] {
					t.Errorf("at offset %d got %q, want %q",
						offsets[idx], got, expected[idx])
					return
				}
			}
		}()
	}
	wg.Wait()

	for i, off := range offsets {
		entry := cache.entries[off%uint(len(cache.entries))].Load()
		require.NotNil(t, entry,
			"hot offset %d (slot %d) should be admitted after concurrent stress",
			off, off%uint(len(cache.entries)))
		require.Equal(t, off, entry.offset)
		require.Equal(t, expected[i], entry.str)
	}
}

func BenchmarkStringCacheHot(b *testing.B) {
	cache := newStringCache()
	data := []byte("hello world, this is test data")

	for b.Loop() {
		benchmarkStringCacheSink = cache.internAt(0, 5, data)
	}
}

func BenchmarkStringCacheCold(b *testing.B) {
	cache := newStringCache()
	data := make([]byte, 4096+5)
	for i := range data {
		data[i] = 'a' + byte(i%26)
	}

	// These offsets collide in the same slot and alternate, so neither reaches
	// the cache's two-consecutive-miss admission threshold.
	offsets := [...]uint{0, 4096}
	var i uint
	b.ReportAllocs()
	for b.Loop() {
		benchmarkStringCacheSink = cache.internAt(offsets[i%uint(len(offsets))], 5, data)
		i++
	}
}
