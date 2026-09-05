package decoder

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func fieldOffsetMap(key string, target int, value byte) []byte {
	data := make([]byte, target+1+len(key))
	data[0] = 0xe1
	copy(data[1:], ptr(target))
	copy(data[3:], []byte{0xa1, value})
	copy(data[target:], encodedString(key))
	return data
}

func TestFieldOffsetCacheChecksCompleteKeyAcrossDatabases(t *testing.T) {
	type record struct {
		First  uint16 `maxminddb:"abcdefg"`
		Second uint16 `maxminddb:"abXYZfg"`
	}
	const target = 64
	for _, tt := range []struct {
		key    string
		target int
		want   record
	}{
		{"abcdefg", target, record{First: 7}},
		{"abXYZfg", target, record{Second: 7}},
		{"abXYZfg", target * 2, record{Second: 7}},
		{"ab???fg", target, record{}}, // Same fingerprint, but no matching field.
		{"abcdefg", target, record{First: 7}},
	} {
		d := NewWithoutStringCache(fieldOffsetMap(tt.key, tt.target, 7))
		var got record
		require.NoError(t, d.Decode(0, &got))
		require.Equal(t, tt.want, got)
	}
}

func TestFieldOffsetCacheConcurrentCollisions(t *testing.T) {
	type record struct {
		First  uint16 `maxminddb:"first"`
		Second uint16 `maxminddb:"second"`
	}
	data := [][]byte{fieldOffsetMap("first", 64, 1), fieldOffsetMap("second", 128, 2)}
	want := []record{{First: 1}, {Second: 2}}
	var workers sync.WaitGroup
	for worker := range 16 {
		workers.Go(func() {
			i := worker % len(data)
			d := NewWithoutStringCache(data[i])
			for range 1000 {
				var got record
				if err := d.Decode(0, &got); err != nil {
					t.Error(err)
					return
				}
				if got != want[i] {
					t.Errorf("got %+v, want %+v", got, want[i])
					return
				}
			}
		})
	}
	workers.Wait()
}
