package decoder

import (
	"fmt"
	"math/bits"
	"math/rand/v2"
	"runtime"
	"sync"
	"sync/atomic"
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
			got := cache.internAt(tc.offset, data[tc.offset:tc.offset+tc.size])
			require.Equal(t, tc.expected, got)
		}
	}
}

func cachedEntry(cache *stringCache, offset uint) *cacheEntry {
	table := cache.table.Load()
	if table == nil {
		return nil
	}
	bucket := table.bucket(offset)
	// Inspect residents independently of the production candidate filter.
	for slot := range bucket.entries {
		if entry := bucket.entries[slot].Load(); entry != nil && entry.offset == offset {
			return entry
		}
	}
	return nil
}

// Distinct values expose a wrong-offset cache hit.
func stringCacheTestValue(offset uint) []byte {
	return fmt.Appendf(nil, "value-%d", offset)
}

func internTestValue(cache *stringCache, offset uint) string {
	return cache.internAt(offset, stringCacheTestValue(offset))
}

// Keep miss slots distinct unless a test explicitly needs collisions.
func stringCacheTestOffsets(
	t *testing.T,
	count int,
	shareMisses bool,
	accept func(offset uint) bool,
) []uint {
	t.Helper()

	cache := newStringCache()
	used := map[*atomic.Uint64]bool{}
	offsets := make([]uint, 0, count)
	for offset := uint(0); offset < 1<<24 && len(offsets) < count; offset++ {
		if !accept(offset) {
			continue
		}
		if recent := cache.initTable().recentMiss(offset); !shareMisses {
			if used[recent] {
				continue
			}
			used[recent] = true
		}
		offsets = append(offsets, offset)
	}
	require.Len(t, offsets, count, "test setup could not find enough offsets")
	return offsets
}

func sharesFirstBucket(offset uint) bool {
	return (offset>>stringCacheWindowBits)%stringCacheBuckets == 0 &&
		(offset == 0 || offset>>stringCachePassShift != 0)
}

// TestStringCacheTwoMissAdmission verifies the admission policy: the first
// miss at a given offset records the offset in recentMisses but does not
// store an entry; the second miss at the same offset admits the entry; the
// third call hits the cache and returns the admitted string verbatim.
func TestStringCacheTwoMissAdmission(t *testing.T) {
	cache := newStringCache()
	data := []byte("hello world, this is test data")

	str1 := cache.internAt(0, data[:5])
	require.Equal(t, "hello", str1)
	require.Nil(t, cachedEntry(cache, 0),
		"first miss must not admit (one-off offsets should not allocate cache slots)")
	require.Equal(t, uint64(stringCacheAdmissionValue(0)), cache.initTable().recentMiss(0).Load(),
		"first miss must record the offset")

	str2 := cache.internAt(0, data[:5])
	require.Equal(t, "hello", str2)
	entry := cachedEntry(cache, 0)
	require.NotNil(t, entry, "second miss at same offset must admit")
	require.Equal(t, uint(0), entry.offset)
	require.Equal(t, "hello", entry.str)
	require.Zero(t, cache.initTable().recentMiss(0).Load(),
		"admission must release the recorded miss")

	str3 := cache.internAt(0, data[:5])
	require.Equal(t, "hello", str3)
	require.Equal(t,
		//nolint:gosec // test only
		unsafe.StringData(entry.str), unsafe.StringData(str3),
		"cache hit must return the admitted string's backing data, not a fresh allocation")
}

// Share a miss slot without contending for the same bucket.
func stringCacheOffsetsSharingMissSlot(t *testing.T, cache *stringCache, count uint) []uint {
	t.Helper()

	offsets := make([]uint, count)
	for i := range offsets {
		offsets[i] = uint(i) * stringCacheMisses << stringCacheMissShift
		require.Same(
			t,
			cache.initTable().recentMiss(offsets[0]),
			cache.initTable().recentMiss(offsets[i]),
		)
		if i > 0 {
			require.NotSame(
				t,
				cache.initTable().bucket(offsets[i-1]),
				cache.initTable().bucket(offsets[i]),
			)
		}
	}
	return offsets
}

func TestStringCacheAdmitsOffsetsSharingMissSlot(t *testing.T) {
	cache := newStringCache()
	offsets := stringCacheOffsetsSharingMissSlot(t, cache, 2)

	for range 2 {
		for _, offset := range offsets {
			require.Equal(t,
				string(stringCacheTestValue(offset)),
				internTestValue(cache, offset),
			)
		}
	}

	requireServedFromCache(t, cache, offsets)
	require.Zero(t, cache.initTable().recentMiss(offsets[0]).Load(),
		"admission must release both recorded misses")
}

func TestStringCacheForgetsOldestMiss(t *testing.T) {
	cache := newStringCache()
	offsets := stringCacheOffsetsSharingMissSlot(t, cache, 3)
	oldest, recent := offsets[0], offsets[1:]

	for _, offset := range offsets {
		internTestValue(cache, offset)
	}

	internTestValue(cache, oldest)
	require.Nil(t, cachedEntry(cache, oldest),
		"a miss that the slot no longer remembers must not admit")

	internTestValue(cache, recent[1])
	require.NotNil(t, cachedEntry(cache, recent[1]))
	internTestValue(cache, recent[0])
	require.Nil(t, cachedEntry(cache, recent[0]))
}

// These sizes keep each bucket in one cache line on 64-bit systems.
func TestStringCacheLayout(t *testing.T) {
	if bits.UintSize != 64 {
		t.Skip("the cache is sized for 64-bit platforms")
	}
	require.Equal(t, uintptr(64), unsafe.Sizeof(cacheBucket{}))
	require.Equal(t, uintptr(72<<10), unsafe.Sizeof(stringCacheTable{}))

	var cache stringCacheTable
	reserved := unsafe.Offsetof(cache.recentMisses) - unsafe.Offsetof(cache.searching)
	require.Equal(t, uintptr(128), reserved,
		"searching must not share a cache line with recentMisses")
}

func TestStringCacheHomeSlots(t *testing.T) {
	const window = 1 << stringCacheWindowBits
	for first := range uint(window) {
		require.Less(t, stringCacheHomeSlot(first), uint(stringCacheBucketSlots))
		for second := first + 3; second < window; second++ {
			require.NotEqual(t,
				stringCacheHomeSlot(first),
				stringCacheHomeSlot(second),
				"positions %d and %d must not share a home slot", first, second,
			)
		}
	}

	require.Equal(t, stringCacheHomeSlot(5), stringCacheHomeSlot(5+window))
	require.Equal(t, stringCacheHomeSlot(5), stringCacheHomeSlot(5+1<<stringCachePassShift))
}

func TestStringCacheKeepsOffsetsSharingBucket(t *testing.T) {
	cache := newStringCache()
	offsets := stringCacheTestOffsets(t, stringCacheBucketSlots, false, sharesFirstBucket)

	for range 3 {
		for _, offset := range offsets {
			require.Equal(t,
				string(stringCacheTestValue(offset)),
				internTestValue(cache, offset),
			)
		}
	}
	for _, offset := range offsets {
		entry := cachedEntry(cache, offset)
		require.NotNil(t, entry, "offset %d must keep its own slot in the shared bucket", offset)
		require.Equal(t, string(stringCacheTestValue(offset)), entry.str)
	}
}

func TestStringCacheConfirmsOffsetWhenControlBytesMatch(t *testing.T) {
	cache := newStringCache()
	offsets := stringCacheTestOffsets(t, 2, true, func(offset uint) bool {
		return cache.initTable().bucket(offset) == cache.initTable().bucket(0) &&
			stringCacheControlByte(offset) == stringCacheControlByte(0)
	})

	// Sequential admission avoids contention in their shared miss slot.
	for _, offset := range offsets {
		for range 3 {
			require.Equal(t,
				string(stringCacheTestValue(offset)),
				internTestValue(cache, offset),
			)
		}
	}
	for range 2 {
		for _, offset := range offsets {
			entry := cachedEntry(cache, offset)
			require.NotNil(t, entry)
			require.Equal(t, offset, entry.offset)

			got := internTestValue(cache, offset)
			require.Equal(t, string(stringCacheTestValue(offset)), got)
			require.Equal(t,
				//nolint:gosec // test only
				unsafe.StringData(entry.str), unsafe.StringData(got),
				"each offset must be served from its own entry")
		}
	}
}

func TestStringCacheEvictsUnusedEntry(t *testing.T) {
	cache := newStringCache()
	offsets := stringCacheTestOffsets(t, stringCacheBucketSlots+1, false, sharesFirstBucket)
	residents, newcomer := offsets[:stringCacheBucketSlots], offsets[stringCacheBucketSlots]

	for _, offset := range residents {
		internTestValue(cache, offset)
		internTestValue(cache, offset)
		require.NotNil(t, cachedEntry(cache, offset))
	}

	internTestValue(cache, newcomer)
	require.Nil(t, cachedEntry(cache, newcomer), "first miss must not admit")
	internTestValue(cache, newcomer)
	require.Nil(t, cachedEntry(cache, newcomer),
		"a newcomer must not displace entries that all count as used")
	for _, offset := range residents {
		require.NotNil(t, cachedEntry(cache, offset))
	}

	internTestValue(cache, newcomer)
	require.NotNil(t, cachedEntry(cache, newcomer),
		"the newcomer's recorded miss must survive the failed attempt")

	remaining := 0
	for _, offset := range residents {
		if cachedEntry(cache, offset) != nil {
			remaining++
		}
	}
	require.Equal(t, stringCacheBucketSlots-1, remaining, "exactly one entry must be evicted")
}

func TestStringCacheKeepsUsedEntries(t *testing.T) {
	cache := newStringCache()
	offsets := stringCacheTestOffsets(t, stringCacheBucketSlots+1, false, sharesFirstBucket)
	residents, newcomer := offsets[:stringCacheBucketSlots], offsets[stringCacheBucketSlots]

	for _, offset := range residents {
		internTestValue(cache, offset)
		internTestValue(cache, offset)
	}

	internTestValue(cache, newcomer)
	internTestValue(cache, newcomer)
	require.Nil(t, cachedEntry(cache, newcomer))

	// Refresh all but one resident to select the eviction victim.
	idle, active := residents[0], residents[1:]
	for _, offset := range active {
		internTestValue(cache, offset)
	}

	require.Equal(t, string(stringCacheTestValue(newcomer)), internTestValue(cache, newcomer))
	require.NotNil(t, cachedEntry(cache, newcomer))
	require.Nil(t, cachedEntry(cache, idle), "the idle entry must be the one evicted")
	for _, offset := range active {
		require.NotNil(t, cachedEntry(cache, offset), "entries in use must survive")
	}
}

func requireServedFromCache(t *testing.T, cache *stringCache, offsets []uint) {
	t.Helper()
	for _, offset := range offsets {
		entry := cachedEntry(cache, offset)
		require.NotNil(t, entry, "offset %d must be cached", offset)

		got := internTestValue(cache, offset)
		require.Equal(t, string(stringCacheTestValue(offset)), got)
		require.Equal(t,
			//nolint:gosec // test only
			unsafe.StringData(entry.str), unsafe.StringData(got),
			"offset %d must be served from its own entry", offset)
	}
}

func TestStringCacheReadsHomeSlotsUntilDisplacement(t *testing.T) {
	cache := newStringCache()

	var neighbors []uint
	for offset := uint(0); offset < 64<<stringCacheWindowBits; offset += 4 {
		neighbors = append(neighbors, offset)
	}
	for _, offset := range neighbors {
		internTestValue(cache, offset)
		internTestValue(cache, offset)

		entry := cache.initTable().bucket(offset).entries[stringCacheHomeSlot(offset)].Load()
		require.NotNil(t, entry)
		require.Equal(t, offset, entry.offset, "offset %d must be in its home slot", offset)
	}
	require.False(t, cache.initTable().searching.Load())
	requireServedFromCache(t, cache, neighbors)

	for offset := uint(1 << 20); offset < 1<<20+4096; offset += 5 {
		internTestValue(cache, offset)
	}
	require.False(t, cache.initTable().searching.Load())

	// A later table pass collides with the first entry's home slot.
	displaced := neighbors[0] + 1<<stringCachePassShift
	require.Same(t, cache.initTable().bucket(neighbors[0]), cache.initTable().bucket(displaced))
	require.Equal(t, stringCacheHomeSlot(neighbors[0]), stringCacheHomeSlot(displaced))

	internTestValue(cache, displaced)
	internTestValue(cache, displaced)
	require.True(t, cache.initTable().searching.Load())

	requireServedFromCache(t, cache, append(neighbors, displaced))
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
	for range goroutines {
		wg.Go(func() {
			for iter := range iterations {
				idx := iter % len(offsets)
				offset := offsets[idx]
				got := cache.internAt(offset, data[offset:offset+size])
				if got != expected[idx] {
					t.Errorf("at offset %d got %q, want %q",
						offsets[idx], got, expected[idx])
					return
				}
			}
		})
	}
	wg.Wait()

	for i, off := range offsets {
		entry := cachedEntry(cache, off)
		require.NotNil(t, entry,
			"hot offset %d should be admitted after concurrent stress", off)
		require.Equal(t, off, entry.offset)
		require.Equal(t, expected[i], entry.str)
	}
}

func TestStringCacheConcurrentEviction(t *testing.T) {
	cache := newStringCache()

	crowded := stringCacheTestOffsets(t, 3*stringCacheBucketSlots, true, sharesFirstBucket)
	offsets := append([]uint(nil), crowded...)
	for offset := range uint(4 * stringCacheBuckets * stringCacheBucketSlots) {
		offsets = append(offsets, offset*3)
	}

	const goroutines = 16
	const iterations = 50_000
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Go(func() {
			//nolint:gosec // this is a test
			r := rand.New(rand.NewPCG(uint64(g), 1))
			for range iterations {
				var offset uint
				if r.IntN(2) == 0 {
					offset = crowded[r.IntN(len(crowded))]
				} else {
					offset = offsets[r.IntN(len(offsets))]
				}
				want := stringCacheTestValue(offset)
				if got := cache.internAt(offset, want); got != string(want) {
					t.Errorf("at offset %d got %q, want %q", offset, got, want)
					return
				}
			}
		})
	}
	wg.Wait()

	for i := range cache.initTable().buckets {
		bucket := &cache.initTable().buckets[i]
		control := bucket.control.Load()
		require.Zero(t, control&stringCacheWriting, "bucket writer did not finish")
		for slot := range bucket.entries {
			entry := bucket.entries[slot].Load()
			if entry == nil {
				require.Zero(t, control&(0xFF<<(8*slot)), "occupied slot has no entry")
				continue
			}
			require.Equal(t, string(stringCacheTestValue(entry.offset)), entry.str)
			require.Same(t, bucket, cache.initTable().bucket(entry.offset),
				"offset %d is stored in a bucket that lookups never search", entry.offset)
			require.Equal(t, stringCacheControlByte(entry.offset),
				(control>>(8*slot))&0xFF,
				"slot's control byte must describe its published entry")
		}
	}
}

func BenchmarkStringCacheHotHome(b *testing.B) {
	benchmarkStringCacheHot(b, false)
}

func BenchmarkStringCacheHotDisplaced(b *testing.B) {
	benchmarkStringCacheHot(b, true)
}

func benchmarkStringCacheHot(b *testing.B, displaced bool) {
	cache := newStringCache()
	data := []byte("hello world, this is test data")
	var offset uint
	for range 2 {
		cache.internAt(offset, data[:5])
	}
	if displaced {
		// A second entry with the same home slot enables bucket scans.
		offset = 1 << stringCachePassShift
		for range 2 {
			cache.internAt(offset, data[:5])
		}
	}

	b.ReportAllocs()
	metrics := startStringCacheBenchmarkMetrics()
	for b.Loop() {
		benchmarkStringCacheSink = cache.internAt(offset, data[:5])
	}
	b.StopTimer()
	metrics.report(b)
}

func BenchmarkStringCacheColdMillionOffsets(b *testing.B) {
	cache := newStringCache()
	const universe = benchmarkStringCacheUniverse
	data := make([]byte, universe*benchmarkStringCacheSpacing+benchmarkStringCacheLength)
	for i := range data {
		data[i] = 'a' + byte(i%26)
	}

	// One pass evicts the prior miss record before an offset returns.
	var i uint
	b.ReportAllocs()
	metrics := startStringCacheBenchmarkMetrics()
	for b.Loop() {
		offset := (i % universe) * benchmarkStringCacheSpacing
		benchmarkStringCacheSink = cache.internAt(
			offset,
			data[offset:offset+benchmarkStringCacheLength],
		)
		i++
	}
	b.StopTimer()
	metrics.report(b)
}

const (
	benchmarkStringCacheSpacing  = 13
	benchmarkStringCacheLength   = 8
	benchmarkStringCacheSteps    = 1 << 20
	benchmarkStringCacheUniverse = 1 << 20
)

// stringCacheWorkload spreads hot strings across a larger universe to test
// collisions rather than an artificially packed hot set.
type stringCacheWorkload struct {
	data    []byte
	offsets []uint32
}

func newStringCacheWorkload(hot, hotPercent int) *stringCacheWorkload {
	data := make(
		[]byte,
		benchmarkStringCacheUniverse*benchmarkStringCacheSpacing+benchmarkStringCacheLength,
	)
	for i := range data {
		data[i] = 'a' + byte(i%26)
	}

	//nolint:gosec // this is a test
	r := rand.New(rand.NewPCG(1, uint64(hot)))
	order := r.Perm(benchmarkStringCacheUniverse)
	workload := &stringCacheWorkload{
		data:    data,
		offsets: make([]uint32, benchmarkStringCacheSteps),
	}
	for i := range workload.offsets {
		var index int
		if r.IntN(100) < hotPercent {
			index = order[r.IntN(hot)]
		} else {
			index = order[hot+r.IntN(len(order)-hot)]
		}
		workload.offsets[i] = uint32(index * benchmarkStringCacheSpacing)
	}
	return workload
}

func (w *stringCacheWorkload) intern(cache *stringCache, step uint) string {
	offset := uint(w.offsets[step&(benchmarkStringCacheSteps-1)])
	return cache.internAt(offset, w.data[offset:offset+benchmarkStringCacheLength])
}

func (w *stringCacheWorkload) warm(cache *stringCache) {
	for step := range uint(2 * benchmarkStringCacheSteps) {
		w.intern(cache, step)
	}
}

// The hot set recurs; cold reads are unlikely to recur before eviction.
var stringCacheWorkloads = []struct {
	name       string
	hot        int
	hotPercent int
}{
	{name: "hot_16", hot: 16, hotPercent: 100},
	{name: "hot_64", hot: 64, hotPercent: 100},
	{name: "hot_256", hot: 256, hotPercent: 100},
	{name: "hot_1024", hot: 1024, hotPercent: 100},
	{name: "hot_2048", hot: 2048, hotPercent: 100},
	{name: "hot_3072", hot: 3072, hotPercent: 100},
	{name: "hot_4096", hot: 4096, hotPercent: 100},
	{name: "hot_6144", hot: 6144, hotPercent: 100},
	{name: "cold", hot: 1, hotPercent: 0},
	{name: "hot_2048_with_20pct_cold", hot: 2048, hotPercent: 80},
	{name: "hot_3072_with_50pct_cold", hot: 3072, hotPercent: 50},
}

func BenchmarkStringCacheWorkload(b *testing.B) {
	for _, tc := range stringCacheWorkloads {
		workload := newStringCacheWorkload(tc.hot, tc.hotPercent)
		b.Run(tc.name, func(b *testing.B) {
			cache := newStringCache()
			workload.warm(cache)

			var step uint
			b.ReportAllocs()
			metrics := startStringCacheBenchmarkMetrics()
			for b.Loop() {
				benchmarkStringCacheSink = workload.intern(cache, step)
				step++
			}
			b.StopTimer()
			metrics.report(b)
		})
	}
}

func BenchmarkStringCacheWorkloadParallel(b *testing.B) {
	for _, tc := range stringCacheWorkloads {
		workload := newStringCacheWorkload(tc.hot, tc.hotPercent)
		b.Run(tc.name, func(b *testing.B) {
			cache := newStringCache()
			workload.warm(cache)

			var starts atomic.Uint64
			b.ReportAllocs()
			metrics := startStringCacheBenchmarkMetrics()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				// Stagger workers so they do not walk the sequence in lockstep.
				step := uint(starts.Add(1)) * 104729
				var length int
				for pb.Next() {
					length += len(workload.intern(cache, step))
					step++
				}
				runtime.KeepAlive(length)
			})
			b.StopTimer()
			metrics.report(b)
		})
	}
}
