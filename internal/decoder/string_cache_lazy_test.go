package decoder

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLazyCacheAllocationEligibility(t *testing.T) {
	for _, value := range []string{"", "x", "ab", "hello", strings.Repeat("x", 100), strings.Repeat("x", 101)} {
		t.Run(fmt.Sprintf("length_%d", len(value)), func(t *testing.T) {
			var data []byte
			if len(value) < 29 {
				data = encodedString(value)
			} else {
				data = append([]byte{0x5d, byte(len(value) - 29)}, value...)
			}
			d := NewDataDecoder(data)
			require.Nil(
				t,
				d.stringCache.table.Load(),
				"cache table allocated before an eligible string",
			)
			got, _, err := d.decodeStringValue(0)
			require.NoError(t, err)
			require.Equal(t, value, got)
			eligible := len(value) >= 2 && len(value) <= 100
			require.Equal(t, eligible, d.stringCache.table.Load() != nil)
		})
	}
	d := NewDataDecoder([]byte{0x45, 'h'})
	_, _, err := d.decodeStringValue(0)
	require.Error(t, err)
	require.Nil(t, d.stringCache.table.Load(), "bounds failure must precede initialization")
	disabled := NewDataDecoderWithoutStringCache(encodedString("hello"))
	_, _, err = disabled.decodeStringValue(0)
	require.NoError(t, err)
	require.Nil(t, disabled.stringCache)
}

func TestLazyCacheConcurrentDecoderCopies(t *testing.T) {
	original := NewDataDecoder(encodedString("hello"))
	require.Nil(t, original.stringCache.table.Load(), "new decoder allocated a cache table")
	const workers = 64
	copies := make([]DataDecoder, workers)
	tables := make([]*stringCacheTable, workers)
	for i := range copies {
		copies[i] = original
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range copies {
		wg.Go(func() {
			<-start
			for range 128 {
				got, _, err := copies[i].decodeStringValue(0)
				if err != nil || got != "hello" {
					t.Errorf("decode: got %q, err %v", got, err)
					return
				}
			}
			tables[i] = copies[i].stringCache.table.Load()
		})
	}
	close(start)
	wg.Wait()
	require.NotNil(t, original.stringCache.table.Load())
	for _, table := range tables {
		require.Same(t, original.stringCache.table.Load(), table)
	}
	require.NotNil(t, cachedEntry(original.stringCache, 0))
}

func TestLazyCacheBooleanDoesNotAllocateTable(t *testing.T) {
	d := NewDataDecoder([]byte{0x01, 0x07})
	value, err := NewDecoder(d, 0).ReadBool()
	require.NoError(t, err)
	require.True(t, value)
	require.Nil(
		t,
		d.stringCache.table.Load(),
		"boolean decode allocated a string cache table",
	)
}
