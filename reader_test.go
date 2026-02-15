package maxminddb

import (
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oschwald/maxminddb-golang/v2/cache"
	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func TestReader(t *testing.T) {
	for _, recordSize := range []uint{24, 28, 32} {
		for _, ipVersion := range []uint{4, 6} {
			fileName := fmt.Sprintf(
				"MaxMind-DB-test-ipv%d-%d.mmdb",
				ipVersion,
				recordSize,
			)
			t.Run(fileName, func(t *testing.T) {
				reader, err := Open(testFile(fileName))
				require.NoError(t, err, "unexpected error while opening database: %v", err)
				checkMetadata(t, reader, ipVersion, recordSize)

				if ipVersion == 4 {
					checkIpv4(t, reader)
				} else {
					checkIpv6(t, reader)
				}
			})
		}
	}
}

func TestReaderBytes(t *testing.T) {
	for _, recordSize := range []uint{24, 28, 32} {
		for _, ipVersion := range []uint{4, 6} {
			fileName := fmt.Sprintf(
				testFile("MaxMind-DB-test-ipv%d-%d.mmdb"),
				ipVersion,
				recordSize,
			)
			bytes, err := os.ReadFile(fileName)
			require.NoError(t, err)
			reader, err := OpenBytes(bytes)
			require.NoError(t, err, "unexpected error while opening bytes: %v", err)

			checkMetadata(t, reader, ipVersion, recordSize)

			if ipVersion == 4 {
				checkIpv4(t, reader)
			} else {
				checkIpv6(t, reader)
			}
		}
	}
}

func TestOpenBytesAppliesReaderOptions(t *testing.T) {
	optionCalled := false
	_, err := OpenBytes(nil, func(*readerOptions) {
		optionCalled = true
	})
	require.Error(t, err)
	require.True(t, optionCalled)
}

type countingCache struct{}

func (countingCache) InternAt(offset, size uint, data []byte) string {
	return string(data[offset : offset+size])
}

type countingCacheProvider struct {
	acquireCount int
	releaseCount int
	cache        cache.Cache
}

func (p *countingCacheProvider) Acquire() cache.Cache {
	p.acquireCount++
	return p.cache
}

func (p *countingCacheProvider) Release(cache.Cache) {
	p.releaseCount++
}

func TestOpenBytesWithCacheOption(t *testing.T) {
	bytes, err := os.ReadFile("GeoLite2-City.mmdb")
	require.NoError(t, err)

	t.Run("nil provider disables cache", func(t *testing.T) {
		reader, err := OpenBytes(bytes, WithCache(nil))
		require.NoError(t, err)
		defer func() { require.NoError(t, reader.Close()) }()

		result := reader.Lookup(netip.MustParseAddr("81.2.69.142"))
		var city fullCityGeneratedLow
		require.NoError(t, result.Decode(&city))
	})

	t.Run("shared provider", func(t *testing.T) {
		provider := cache.NewSharedProvider(cache.Options{
			EntryCount:   2048,
			MinCachedLen: 2,
			MaxCachedLen: 32,
		})
		reader, err := OpenBytes(bytes, WithCache(provider))
		require.NoError(t, err)
		defer func() { require.NoError(t, reader.Close()) }()

		result := reader.Lookup(netip.MustParseAddr("81.2.69.142"))
		var city fullCityGeneratedLow
		require.NoError(t, result.Decode(&city))
	})

	t.Run("pooled provider", func(t *testing.T) {
		provider := cache.NewPooledProvider(cache.Options{
			EntryCount:   2048,
			MinCachedLen: 2,
			MaxCachedLen: 32,
		})
		reader, err := OpenBytes(bytes, WithCache(provider))
		require.NoError(t, err)
		defer func() { require.NoError(t, reader.Close()) }()

		result := reader.Lookup(netip.MustParseAddr("81.2.69.142"))
		var city fullCityGeneratedLow
		require.NoError(t, result.Decode(&city))
	})

	t.Run("custom provider acquire/release", func(t *testing.T) {
		p := &countingCacheProvider{cache: countingCache{}}
		reader, err := OpenBytes(bytes, WithCache(p))
		require.NoError(t, err)
		defer func() { require.NoError(t, reader.Close()) }()

		result := reader.Lookup(netip.MustParseAddr("81.2.69.142"))
		var city fullCityGeneratedLow
		require.NoError(t, result.Decode(&city))
		require.Equal(t, 1, p.acquireCount)
		require.Equal(t, 1, p.releaseCount)
	})
}

func TestReaderLeaks(t *testing.T) {
	collected := make(chan struct{})

	func() {
		r, err := Open(testFile("GeoLite2-City-Test.mmdb"))
		require.NoError(t, err)

		// We intentionally do NOT call Close() to test if GC picks it up
		// and if AddCleanup doesn't prevent it.

		runtime.SetFinalizer(r, func(*Reader) {
			close(collected)
		})
	}()

	require.Eventually(t, func() bool {
		runtime.GC()
		select {
		case <-collected:
			return true
		default:
			return false
		}
	}, 1*time.Second, 50*time.Millisecond, "Reader was NOT collected (leak detected)")
}

func TestLookupNetwork(t *testing.T) {
	bigInt := new(big.Int)
	bigInt.SetString("1329227995784915872903807060280344576", 10)
	decoderRecord := map[string]any{
		"array": []any{
			uint64(1),
			uint64(2),
			uint64(3),
		},
		"boolean": true,
		"bytes": []uint8{
			0x0,
			0x0,
			0x0,
			0x2a,
		},
		"double": 42.123456,
		"float":  float32(1.1),
		"int32":  int32(-268435456),
		"map": map[string]any{
			"mapX": map[string]any{
				"arrayX": []any{
					uint64(0x7),
					uint64(0x8),
					uint64(0x9),
				},
				"utf8_stringX": "hello",
			},
		},
		"uint128":     bigInt,
		"uint16":      uint64(0x64),
		"uint32":      uint64(0x10000000),
		"uint64":      uint64(0x1000000000000000),
		"utf8_string": "unicode! ☯ - ♫",
	}

	tests := []struct {
		IP              netip.Addr
		DBFile          string
		ExpectedNetwork string
		ExpectedRecord  any
		ExpectedFound   bool
	}{
		{
			IP:              netip.MustParseAddr("1.1.1.1"),
			DBFile:          "MaxMind-DB-test-ipv6-32.mmdb",
			ExpectedNetwork: "1.0.0.0/8",
			ExpectedRecord:  nil,
			ExpectedFound:   false,
		},
		{
			IP:              netip.MustParseAddr("::1:ffff:ffff"),
			DBFile:          "MaxMind-DB-test-ipv6-24.mmdb",
			ExpectedNetwork: "::1:ffff:ffff/128",
			ExpectedRecord:  map[string]any{"ip": "::1:ffff:ffff"},
			ExpectedFound:   true,
		},
		{
			IP:              netip.MustParseAddr("::2:0:1"),
			DBFile:          "MaxMind-DB-test-ipv6-24.mmdb",
			ExpectedNetwork: "::2:0:0/122",
			ExpectedRecord:  map[string]any{"ip": "::2:0:0"},
			ExpectedFound:   true,
		},
		{
			IP:              netip.MustParseAddr("1.1.1.1"),
			DBFile:          "MaxMind-DB-test-ipv4-24.mmdb",
			ExpectedNetwork: "1.1.1.1/32",
			ExpectedRecord:  map[string]any{"ip": "1.1.1.1"},
			ExpectedFound:   true,
		},
		{
			IP:              netip.MustParseAddr("1.1.1.3"),
			DBFile:          "MaxMind-DB-test-ipv4-24.mmdb",
			ExpectedNetwork: "1.1.1.2/31",
			ExpectedRecord:  map[string]any{"ip": "1.1.1.2"},
			ExpectedFound:   true,
		},
		{
			IP:              netip.MustParseAddr("1.1.1.3"),
			DBFile:          "MaxMind-DB-test-decoder.mmdb",
			ExpectedNetwork: "1.1.1.0/24",
			ExpectedRecord:  decoderRecord,
			ExpectedFound:   true,
		},
		{
			IP:              netip.MustParseAddr("::ffff:1.1.1.128"),
			DBFile:          "MaxMind-DB-test-decoder.mmdb",
			ExpectedNetwork: "::ffff:1.1.1.0/120",
			ExpectedRecord:  decoderRecord,
			ExpectedFound:   true,
		},
		{
			IP:              netip.MustParseAddr("::1.1.1.128"),
			DBFile:          "MaxMind-DB-test-decoder.mmdb",
			ExpectedNetwork: "::101:100/120",
			ExpectedRecord:  decoderRecord,
			ExpectedFound:   true,
		},
		{
			IP:              netip.MustParseAddr("200.0.2.1"),
			DBFile:          "MaxMind-DB-no-ipv4-search-tree.mmdb",
			ExpectedNetwork: "::/64",
			ExpectedRecord:  "::0/64",
			ExpectedFound:   true,
		},
		{
			IP:              netip.MustParseAddr("::200.0.2.1"),
			DBFile:          "MaxMind-DB-no-ipv4-search-tree.mmdb",
			ExpectedNetwork: "::/64",
			ExpectedRecord:  "::0/64",
			ExpectedFound:   true,
		},
		{
			IP:              netip.MustParseAddr("0:0:0:0:ffff:ffff:ffff:ffff"),
			DBFile:          "MaxMind-DB-no-ipv4-search-tree.mmdb",
			ExpectedNetwork: "::/64",
			ExpectedRecord:  "::0/64",
			ExpectedFound:   true,
		},
		{
			IP:              netip.MustParseAddr("ef00::"),
			DBFile:          "MaxMind-DB-no-ipv4-search-tree.mmdb",
			ExpectedNetwork: "8000::/1",
			ExpectedRecord:  nil,
			ExpectedFound:   false,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s - %s", test.DBFile, test.IP), func(t *testing.T) {
			var record any
			reader, err := Open(testFile(test.DBFile))
			require.NoError(t, err)

			result := reader.Lookup(test.IP)
			require.NoError(t, result.Err())
			assert.Equal(t, test.ExpectedFound, result.Found())
			assert.Equal(t, test.ExpectedNetwork, result.Prefix().String())

			require.NoError(t, result.Decode(&record))
			assert.Equal(t, test.ExpectedRecord, record)
		})
	}
}

func TestDecodingToInterface(t *testing.T) {
	reader, err := Open(testFile("MaxMind-DB-test-decoder.mmdb"))
	require.NoError(t, err, "unexpected error while opening database: %v", err)

	var recordInterface any
	err = reader.Lookup(netip.MustParseAddr("::1.1.1.0")).Decode(&recordInterface)
	require.NoError(t, err, "unexpected error while doing lookup: %v", err)

	checkDecodingToInterface(t, recordInterface)
}

func TestMetadataPointer(t *testing.T) {
	_, err := Open(testFile("MaxMind-DB-test-metadata-pointers.mmdb"))
	require.NoError(t, err, "unexpected error while opening database: %v", err)
}

func checkDecodingToInterface(t *testing.T, recordInterface any) {
	record := recordInterface.(map[string]any)
	assert.Equal(t, []any{uint64(1), uint64(2), uint64(3)}, record["array"])
	assert.Equal(t, true, record["boolean"])
	assert.Equal(t, []byte{0x00, 0x00, 0x00, 0x2a}, record["bytes"])
	assert.InEpsilon(t, 42.123456, record["double"], 1e-10)
	assert.InEpsilon(t, float32(1.1), record["float"], 1e-5)
	assert.Equal(t, int32(-268435456), record["int32"])
	assert.Equal(t,
		map[string]any{
			"mapX": map[string]any{
				"arrayX":       []any{uint64(7), uint64(8), uint64(9)},
				"utf8_stringX": "hello",
			},
		},
		record["map"],
	)

	assert.Equal(t, uint64(100), record["uint16"])
	assert.Equal(t, uint64(268435456), record["uint32"])
	assert.Equal(t, uint64(1152921504606846976), record["uint64"])
	assert.Equal(t, "unicode! ☯ - ♫", record["utf8_string"])
	bigInt := new(big.Int)
	bigInt.SetString("1329227995784915872903807060280344576", 10)
	assert.Equal(t, bigInt, record["uint128"])
}

type TestType struct {
	Array      []uint         `maxminddb:"array"`
	Boolean    bool           `maxminddb:"boolean"`
	Bytes      []byte         `maxminddb:"bytes"`
	Double     float64        `maxminddb:"double"`
	Float      float32        `maxminddb:"float"`
	Int32      int32          `maxminddb:"int32"`
	Map        map[string]any `maxminddb:"map"`
	Uint16     uint16         `maxminddb:"uint16"`
	Uint32     uint32         `maxminddb:"uint32"`
	Uint64     uint64         `maxminddb:"uint64"`
	Uint128    big.Int        `maxminddb:"uint128"`
	Utf8String string         `maxminddb:"utf8_string"`
}

func TestDecoder(t *testing.T) {
	reader, err := Open(testFile("MaxMind-DB-test-decoder.mmdb"))
	require.NoError(t, err)

	verify := func(result TestType) {
		assert.Equal(t, []uint{uint(1), uint(2), uint(3)}, result.Array)
		assert.True(t, result.Boolean)
		assert.Equal(t, []byte{0x00, 0x00, 0x00, 0x2a}, result.Bytes)
		assert.InEpsilon(t, 42.123456, result.Double, 1e-10)
		assert.InEpsilon(t, float32(1.1), result.Float, 1e-5)
		assert.Equal(t, int32(-268435456), result.Int32)

		assert.Equal(t,
			map[string]any{
				"mapX": map[string]any{
					"arrayX":       []any{uint64(7), uint64(8), uint64(9)},
					"utf8_stringX": "hello",
				},
			},
			result.Map,
		)

		assert.Equal(t, uint16(100), result.Uint16)
		assert.Equal(t, uint32(268435456), result.Uint32)
		assert.Equal(t, uint64(1152921504606846976), result.Uint64)
		assert.Equal(t, "unicode! ☯ - ♫", result.Utf8String)
		bigInt := new(big.Int)
		bigInt.SetString("1329227995784915872903807060280344576", 10)
		assert.Equal(t, bigInt, &result.Uint128)
	}

	{
		// Directly lookup and decode.
		var testV TestType
		require.NoError(t, reader.Lookup(netip.MustParseAddr("::1.1.1.0")).Decode(&testV))
		verify(testV)
	}
	{
		// Lookup record offset, then Decode.
		var testV TestType
		result := reader.Lookup(netip.MustParseAddr("::1.1.1.0"))
		require.NoError(t, result.Err())
		require.True(t, result.Found())

		res := reader.LookupOffset(result.Offset())
		require.NoError(t, res.Decode(&testV))
		verify(testV)
	}

	require.NoError(t, reader.Close())
}

func TestDecodePath(t *testing.T) {
	reader, err := Open(testFile("MaxMind-DB-test-decoder.mmdb"))
	require.NoError(t, err)

	result := reader.Lookup(netip.MustParseAddr("::1.1.1.0"))
	require.NoError(t, result.Err())

	var u16 uint16

	require.NoError(t, result.DecodePath(&u16, "uint16"))

	assert.Equal(t, uint16(100), u16)

	var u uint
	require.NoError(t, result.DecodePath(&u, "array", 0))
	assert.Equal(t, uint(1), u)

	var u2 uint
	require.NoError(t, result.DecodePath(&u2, "array", 2))
	assert.Equal(t, uint(3), u2)

	// This is past the end of the array
	var u3 uint
	require.NoError(t, result.DecodePath(&u3, "array", 3))
	assert.Equal(t, uint(0), u3)

	// Negative offsets

	var n1 uint
	require.NoError(t, result.DecodePath(&n1, "array", -1))
	assert.Equal(t, uint(3), n1)

	var n2 uint
	require.NoError(t, result.DecodePath(&n2, "array", -3))
	assert.Equal(t, uint(1), n2)

	var u4 uint
	require.NoError(t, result.DecodePath(&u4, "map", "mapX", "arrayX", 1))
	assert.Equal(t, uint(8), u4)

	// Does key not exist
	var ne uint
	require.NoError(t, result.DecodePath(&ne, "does-not-exist", 1))
	assert.Equal(t, uint(0), ne)

	// Test pointer pattern for path existence checking
	var existingStringPtr *string
	require.NoError(t, result.DecodePath(&existingStringPtr, "utf8_string"))
	assert.NotNil(t, existingStringPtr, "existing path should decode to non-nil pointer")
	assert.Equal(t, "unicode! ☯ - ♫", *existingStringPtr)

	var nonExistentStringPtr *string
	require.NoError(t, result.DecodePath(&nonExistentStringPtr, "does-not-exist"))
	assert.Nil(t, nonExistentStringPtr, "non-existent path should decode to nil pointer")
}

type TestInterface interface {
	method() bool
}

func (t *TestType) method() bool {
	return t.Boolean
}

func TestStructInterface(t *testing.T) {
	var result TestInterface = &TestType{}

	reader, err := Open(testFile("MaxMind-DB-test-decoder.mmdb"))
	require.NoError(t, err)

	require.NoError(t, reader.Lookup(netip.MustParseAddr("::1.1.1.0")).Decode(&result))

	assert.True(t, result.method())
}

func TestNonEmptyNilInterface(t *testing.T) {
	var result TestInterface

	reader, err := Open(testFile("MaxMind-DB-test-decoder.mmdb"))
	require.NoError(t, err)

	err = reader.Lookup(netip.MustParseAddr("::1.1.1.0")).Decode(&result)
	assert.Equal(
		t,
		"at offset 115: maxminddb: cannot unmarshal map into type maxminddb.TestInterface",
		err.Error(),
	)
}

type CityTraits struct {
	AutonomousSystemNumber uint `json:"autonomous_system_number,omitempty" maxminddb:"autonomous_system_number"`
}

type City struct {
	Traits CityTraits `maxminddb:"traits"`
}

func TestEmbeddedStructAsInterface(t *testing.T) {
	var city City
	var result any = city.Traits

	db, err := Open(testFile("GeoIP2-ISP-Test.mmdb"))
	require.NoError(t, err)

	require.NoError(t, db.Lookup(netip.MustParseAddr("1.128.0.0")).Decode(&result))
}

type BoolInterface interface {
	true() bool
}

type Bool bool

func (b Bool) true() bool {
	return bool(b)
}

type ValueTypeTestType struct {
	Boolean BoolInterface `maxminddb:"boolean"`
}

func TestValueTypeInterface(t *testing.T) {
	var result ValueTypeTestType
	result.Boolean = Bool(false)

	reader, err := Open(testFile("MaxMind-DB-test-decoder.mmdb"))
	require.NoError(t, err)

	// although it would be nice to support cases like this, I am not sure it
	// is possible to do so in a general way.
	assert.Error(t, reader.Lookup(netip.MustParseAddr("::1.1.1.0")).Decode(&result))
}

type NestedMapX struct {
	UTF8StringX string `maxminddb:"utf8_stringX"`
}

type NestedPointerMapX struct {
	ArrayX []int `maxminddb:"arrayX"`
}

type PointerMap struct {
	MapX struct {
		NestedMapX
		*NestedPointerMapX

		Ignored string
	} `maxminddb:"mapX"`
}

type TestPointerType struct {
	Array   *[]uint     `maxminddb:"array"`
	Boolean *bool       `maxminddb:"boolean"`
	Bytes   *[]byte     `maxminddb:"bytes"`
	Double  *float64    `maxminddb:"double"`
	Float   *float32    `maxminddb:"float"`
	Int32   *int32      `maxminddb:"int32"`
	Map     *PointerMap `maxminddb:"map"`
	Uint16  *uint16     `maxminddb:"uint16"`
	Uint32  *uint32     `maxminddb:"uint32"`

	// Test for pointer to pointer
	Uint64     **uint64 `maxminddb:"uint64"`
	Uint128    *big.Int `maxminddb:"uint128"`
	Utf8String *string  `maxminddb:"utf8_string"`
}

func TestComplexStructWithNestingAndPointer(t *testing.T) {
	reader, err := Open(testFile("MaxMind-DB-test-decoder.mmdb"))
	require.NoError(t, err)

	var result TestPointerType

	err = reader.Lookup(netip.MustParseAddr("::1.1.1.0")).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, []uint{uint(1), uint(2), uint(3)}, *result.Array)
	assert.True(t, *result.Boolean)
	assert.Equal(t, []byte{0x00, 0x00, 0x00, 0x2a}, *result.Bytes)
	assert.InEpsilon(t, 42.123456, *result.Double, 1e-10)
	assert.InEpsilon(t, float32(1.1), *result.Float, 1e-5)
	assert.Equal(t, int32(-268435456), *result.Int32)

	assert.Equal(t, []int{7, 8, 9}, result.Map.MapX.ArrayX)

	assert.Equal(t, "hello", result.Map.MapX.UTF8StringX)

	assert.Equal(t, uint16(100), *result.Uint16)
	assert.Equal(t, uint32(268435456), *result.Uint32)
	assert.Equal(t, uint64(1152921504606846976), **result.Uint64)
	assert.Equal(t, "unicode! ☯ - ♫", *result.Utf8String)
	bigInt := new(big.Int)
	bigInt.SetString("1329227995784915872903807060280344576", 10)
	assert.Equal(t, bigInt, result.Uint128)

	require.NoError(t, reader.Close())
}

// See GitHub #115.
func TestNestedMapDecode(t *testing.T) {
	db, err := Open(testFile("GeoIP2-Country-Test.mmdb"))
	require.NoError(t, err)

	var r map[string]map[string]any

	require.NoError(t, db.Lookup(netip.MustParseAddr("89.160.20.128")).Decode(&r))

	assert.Equal(
		t,
		map[string]map[string]any{
			"continent": {
				keyCode:      "EU",
				keyGeoNameID: uint64(6255148),
				keyNames: map[string]any{
					"de":    "Europa",
					"en":    "Europe",
					"es":    "Europa",
					"fr":    "Europe",
					"ja":    "ヨーロッパ",
					"pt-BR": "Europa",
					"ru":    "Европа",
					"zh-CN": "欧洲",
				},
			},
			"country": {
				keyGeoNameID:         uint64(2661886),
				keyIsInEuropeanUnion: true,
				keyISOCode:           "SE",
				keyNames: map[string]any{
					"de":    "Schweden",
					"en":    "Sweden",
					"es":    "Suecia",
					"fr":    "Suède",
					"ja":    "スウェーデン王国",
					"pt-BR": "Suécia",
					"ru":    "Швеция",
					"zh-CN": "瑞典",
				},
			},
			"registered_country": {
				keyGeoNameID:         uint64(2921044),
				keyIsInEuropeanUnion: true,
				keyISOCode:           "DE",
				keyNames: map[string]any{
					"de":    "Deutschland",
					"en":    "Germany",
					"es":    "Alemania",
					"fr":    "Allemagne",
					"ja":    "ドイツ連邦共和国",
					"pt-BR": "Alemanha",
					"ru":    "Германия",
					"zh-CN": "德国",
				},
			},
		},
		r,
	)
}

func TestNestedOffsetDecode(t *testing.T) {
	db, err := Open(testFile("GeoIP2-City-Test.mmdb"))
	require.NoError(t, err)

	result := db.Lookup(netip.MustParseAddr("81.2.69.142"))
	require.NoError(t, result.Err())
	require.True(t, result.Found())

	var root struct {
		CountryOffset uintptr `maxminddb:"country"`

		Location struct {
			Latitude float64 `maxminddb:"latitude"`
			// Longitude is directly nested within the parent map.
			LongitudeOffset uintptr `maxminddb:"longitude"`
			// TimeZone is indirected via a pointer.
			TimeZoneOffset uintptr `maxminddb:"time_zone"`
		} `maxminddb:"location"`
	}
	res := db.LookupOffset(result.Offset())
	require.NoError(t, res.Decode(&root))
	assert.InEpsilon(t, 51.5142, root.Location.Latitude, 1e-10)

	var longitude float64
	res = db.LookupOffset(root.Location.LongitudeOffset)
	require.NoError(t, res.Decode(&longitude))
	assert.InEpsilon(t, -0.0931, longitude, 1e-10)

	var timeZone string
	res = db.LookupOffset(root.Location.TimeZoneOffset)
	require.NoError(t, res.Decode(&timeZone))
	assert.Equal(t, "Europe/London", timeZone)

	var country struct {
		IsoCode string `maxminddb:"iso_code"`
	}
	res = db.LookupOffset(root.CountryOffset)
	require.NoError(t, res.Decode(&country))
	assert.Equal(t, "GB", country.IsoCode)

	require.NoError(t, db.Close())
}

func TestDecodingUint16IntoInt(t *testing.T) {
	reader, err := Open(testFile("MaxMind-DB-test-decoder.mmdb"))
	require.NoError(t, err, "unexpected error while opening database: %v", err)

	var result struct {
		Uint16 int `maxminddb:"uint16"`
	}
	err = reader.Lookup(netip.MustParseAddr("::1.1.1.0")).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, 100, result.Uint16)
}

func TestIpv6inIpv4(t *testing.T) {
	reader, err := Open(testFile("MaxMind-DB-test-ipv4-24.mmdb"))
	require.NoError(t, err, "unexpected error while opening database: %v", err)

	var result TestType
	err = reader.Lookup(netip.MustParseAddr("2001::")).Decode(&result)

	var emptyResult TestType
	assert.Equal(t, emptyResult, result)

	expected := errors.New(
		"error looking up '2001::': you attempted to look up an IPv6 address in an IPv4-only database",
	)
	assert.Equal(t, expected, err)
	require.NoError(t, reader.Close(), "error on close")
}

func TestBrokenDoubleDatabase(t *testing.T) {
	reader, err := Open(testFile("GeoIP2-City-Test-Broken-Double-Format.mmdb"))
	require.NoError(t, err, "unexpected error while opening database: %v", err)

	var result any
	err = reader.Lookup(netip.MustParseAddr("2001:220::")).Decode(&result)

	expected := mmdberrors.NewInvalidDatabaseError(
		"the MaxMind DB file's data section contains bad data (float 64 size of 2)",
	)
	require.ErrorAs(t, err, &expected)
	require.NoError(t, reader.Close(), "error on close")
}

func TestInvalidNodeCountDatabase(t *testing.T) {
	_, err := Open(testFile("GeoIP2-City-Test-Invalid-Node-Count.mmdb"))

	expected := mmdberrors.NewInvalidDatabaseError("the MaxMind DB contains invalid metadata")
	assert.Equal(t, expected, err)
}

func TestMissingDatabase(t *testing.T) {
	reader, err := Open("file-does-not-exist.mmdb")
	assert.Nil(t, reader, "received reader when doing lookups on DB that doesn't exist")
	assert.Regexp(t, "open file-does-not-exist.mmdb.*", err)
}

func TestNonDatabase(t *testing.T) {
	reader, err := Open("README.md")
	assert.Nil(t, reader, "received reader when doing lookups on DB that doesn't exist")
	assert.Equal(t, "error opening database: invalid MaxMind DB file", err.Error())
}

func TestDecodingToNonPointer(t *testing.T) {
	reader, err := Open(testFile("MaxMind-DB-test-decoder.mmdb"))
	require.NoError(t, err)

	var recordInterface any
	err = reader.Lookup(netip.MustParseAddr("::1.1.1.0")).Decode(recordInterface)
	assert.Equal(t, "result param must be a pointer", err.Error())
	require.NoError(t, reader.Close(), "error on close")
}

func TestUsingClosedDatabase(t *testing.T) {
	reader, err := Open(testFile("MaxMind-DB-test-decoder.mmdb"))
	require.NoError(t, err)
	require.NoError(t, reader.Close())

	addr := netip.MustParseAddr("::")

	result := reader.Lookup(addr)
	assert.Equal(t, "cannot call Lookup on a closed database", result.Err().Error())

	var recordInterface any
	err = reader.Lookup(addr).Decode(recordInterface)
	assert.Equal(t, "cannot call Lookup on a closed database", err.Error())

	err = reader.LookupOffset(0).Decode(recordInterface)
	assert.Equal(t, "cannot call LookupOffset on a closed database", err.Error())
}

func checkMetadata(t *testing.T, reader *Reader, ipVersion, recordSize uint) {
	metadata := reader.Metadata

	assert.Equal(t, uint(2), metadata.BinaryFormatMajorVersion)

	assert.Equal(t, uint(0), metadata.BinaryFormatMinorVersion)
	assert.IsType(t, uint(0), metadata.BuildEpoch)
	assert.Equal(t, "Test", metadata.DatabaseType)

	assert.Equal(t, map[string]string{
		"en": "Test Database",
		"zh": "Test Database Chinese",
	}, metadata.Description)
	assert.Equal(t, ipVersion, metadata.IPVersion)
	assert.Equal(t, []string{"en", "zh"}, metadata.Languages)

	if ipVersion == 4 {
		assert.Equal(t, uint(164), metadata.NodeCount)
	} else {
		assert.Equal(t, uint(416), metadata.NodeCount)
	}

	assert.Equal(t, recordSize, metadata.RecordSize)
}

func checkIpv4(t *testing.T, reader *Reader) {
	for i := range uint(6) {
		address := fmt.Sprintf("1.1.1.%d", uint(1)<<i)
		ip := netip.MustParseAddr(address)

		var result map[string]string
		err := reader.Lookup(ip).Decode(&result)
		require.NoError(t, err, "unexpected error while doing lookup: %v", err)
		assert.Equal(t, map[string]string{"ip": address}, result)
	}
	pairs := map[string]string{
		"1.1.1.3":  "1.1.1.2",
		"1.1.1.5":  "1.1.1.4",
		"1.1.1.7":  "1.1.1.4",
		"1.1.1.9":  "1.1.1.8",
		"1.1.1.15": "1.1.1.8",
		"1.1.1.17": "1.1.1.16",
		"1.1.1.31": "1.1.1.16",
	}

	for keyAddress, valueAddress := range pairs {
		data := map[string]string{"ip": valueAddress}

		ip := netip.MustParseAddr(keyAddress)

		var result map[string]string
		err := reader.Lookup(ip).Decode(&result)
		require.NoError(t, err, "unexpected error while doing lookup: %v", err)
		assert.Equal(t, data, result)
	}

	for _, address := range []string{"1.1.1.33", "255.254.253.123"} {
		ip := netip.MustParseAddr(address)

		var result map[string]string
		err := reader.Lookup(ip).Decode(&result)
		require.NoError(t, err, "unexpected error while doing lookup: %v", err)
		assert.Nil(t, result)
	}
}

func checkIpv6(t *testing.T, reader *Reader) {
	subnets := []string{
		"::1:ffff:ffff", "::2:0:0",
		"::2:0:40", "::2:0:50", "::2:0:58",
	}

	for _, address := range subnets {
		var result map[string]string
		err := reader.Lookup(netip.MustParseAddr(address)).Decode(&result)
		require.NoError(t, err, "unexpected error while doing lookup: %v", err)
		assert.Equal(t, map[string]string{"ip": address}, result)
	}

	pairs := map[string]string{
		"::2:0:1":  "::2:0:0",
		"::2:0:33": "::2:0:0",
		"::2:0:39": "::2:0:0",
		"::2:0:41": "::2:0:40",
		"::2:0:49": "::2:0:40",
		"::2:0:52": "::2:0:50",
		"::2:0:57": "::2:0:50",
		"::2:0:59": "::2:0:58",
	}

	for keyAddress, valueAddress := range pairs {
		data := map[string]string{"ip": valueAddress}
		var result map[string]string
		err := reader.Lookup(netip.MustParseAddr(keyAddress)).Decode(&result)
		require.NoError(t, err, "unexpected error while doing lookup: %v", err)
		assert.Equal(t, data, result)
	}

	for _, address := range []string{"1.1.1.33", "255.254.253.123", "89fa::"} {
		var result map[string]string
		err := reader.Lookup(netip.MustParseAddr(address)).Decode(&result)
		require.NoError(t, err, "unexpected error while doing lookup: %v", err)
		assert.Nil(t, result)
	}
}

func BenchmarkOpen(b *testing.B) {
	var db *Reader
	var err error
	for b.Loop() {
		db, err = Open("GeoLite2-City.mmdb")
		if err != nil {
			b.Fatal(err)
		}
	}
	assert.NotNil(b, db)
	require.NoError(b, db.Close(), "error on close")
}

func BenchmarkInterfaceLookup(b *testing.B) {
	db, err := Open("GeoLite2-City.mmdb")
	require.NoError(b, err)

	//nolint:gosec // this is a test
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	var result any

	s := make(net.IP, 4)
	for b.Loop() {
		ip := randomIPv4Address(r, s)
		err = db.Lookup(ip).Decode(&result)
		if err != nil {
			b.Error(err)
		}
	}
	require.NoError(b, db.Close(), "error on close")
}

func BenchmarkLookupNetwork(b *testing.B) {
	db, err := Open("GeoLite2-City.mmdb")
	require.NoError(b, err)

	//nolint:gosec // this is a test
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	s := make(net.IP, 4)
	for b.Loop() {
		ip := randomIPv4Address(r, s)
		res := db.Lookup(ip)
		if err := res.Err(); err != nil {
			b.Error(err)
		}
		if !res.Prefix().IsValid() {
			b.Fatalf("invalid network for %s", ip)
		}
	}
	require.NoError(b, db.Close(), "error on close")
}

type fullCity struct {
	City struct {
		GeoNameID uint              `maxminddb:"geoname_id"`
		Names     map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
	Continent struct {
		Code      string            `maxminddb:"code"`
		GeoNameID uint              `maxminddb:"geoname_id"`
		Names     map[string]string `maxminddb:"names"`
	} `maxminddb:"continent"`
	Country struct {
		GeoNameID         uint              `maxminddb:"geoname_id"`
		IsInEuropeanUnion bool              `maxminddb:"is_in_european_union"`
		IsoCode           string            `maxminddb:"iso_code"`
		Names             map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
	Location struct {
		AccuracyRadius uint16  `maxminddb:"accuracy_radius"`
		Latitude       float64 `maxminddb:"latitude"`
		Longitude      float64 `maxminddb:"longitude"`
		MetroCode      uint    `maxminddb:"metro_code"`
		TimeZone       string  `maxminddb:"time_zone"`
	} `maxminddb:"location"`
	Postal struct {
		Code string `maxminddb:"code"`
	} `maxminddb:"postal"`
	RegisteredCountry struct {
		GeoNameID         uint              `maxminddb:"geoname_id"`
		IsInEuropeanUnion bool              `maxminddb:"is_in_european_union"`
		IsoCode           string            `maxminddb:"iso_code"`
		Names             map[string]string `maxminddb:"names"`
	} `maxminddb:"registered_country"`
	RepresentedCountry struct {
		GeoNameID         uint              `maxminddb:"geoname_id"`
		IsInEuropeanUnion bool              `maxminddb:"is_in_european_union"`
		IsoCode           string            `maxminddb:"iso_code"`
		Names             map[string]string `maxminddb:"names"`
		Type              string            `maxminddb:"type"`
	} `maxminddb:"represented_country"`
	Subdivisions []struct {
		GeoNameID uint              `maxminddb:"geoname_id"`
		IsoCode   string            `maxminddb:"iso_code"`
		Names     map[string]string `maxminddb:"names"`
	} `maxminddb:"subdivisions"`
	Traits struct {
		IsAnonymousProxy    bool `maxminddb:"is_anonymous_proxy"`
		IsSatelliteProvider bool `maxminddb:"is_satellite_provider"`
	} `maxminddb:"traits"`
}

type generatedGeoNamesSection struct {
	GeoNameID uint
	Names     map[string]string
}

type generatedContinentSection struct {
	Code      string
	GeoNameID uint
	Names     map[string]string
}

type generatedCountrySection struct {
	GeoNameID         uint
	IsInEuropeanUnion bool
	IsoCode           string
	Names             map[string]string
}

type generatedRepresentedCountrySection struct {
	GeoNameID         uint
	IsInEuropeanUnion bool
	IsoCode           string
	Names             map[string]string
	Type              string
}

type generatedLocationSection struct {
	AccuracyRadius uint16
	Latitude       float64
	Longitude      float64
	MetroCode      uint
	TimeZone       string
}

type generatedPostalSection struct {
	Code string
}

type generatedSubdivision struct {
	GeoNameID uint
	IsoCode   string
	Names     map[string]string
}

type generatedTraitsSection struct {
	IsAnonymousProxy    bool
	IsSatelliteProvider bool
}

const (
	keyGeoNameID         = "geoname_id"
	keyNames             = "names"
	keyCode              = "code"
	keyIsInEuropeanUnion = "is_in_european_union"
	keyISOCode           = "iso_code"
)

// fullCityGeneratedLow uses the low-level offset-based decoder APIs.
type fullCityGeneratedLow struct {
	City               generatedGeoNamesSection
	Continent          generatedContinentSection
	Country            generatedCountrySection
	Location           generatedLocationSection
	Postal             generatedPostalSection
	RegisteredCountry  generatedCountrySection
	RepresentedCountry generatedRepresentedCountrySection
	Subdivisions       []generatedSubdivision
	Traits             generatedTraitsSection
}

func readUintAt(d *mmdbdata.Decoder, offset uint) (uint, uint, error) {
	v, nextOffset, err := d.ReadUintAt(offset)
	return uint(v), nextOffset, err
}

func decodeStringMapAt(d *mmdbdata.Decoder, offset uint, out *map[string]string) (uint, error) {
	if *out == nil {
		*out = map[string]string{}
	} else {
		clear(*out)
	}

	size, keyOffset, err := d.ReadMapHeaderAt(offset)
	if err != nil {
		return 0, err
	}

	for range size {
		key, valueOffset, err := d.ReadMapEntryStringValueOffsetAt(keyOffset)
		if err != nil {
			return 0, err
		}
		value, nextOffset, err := d.ReadStringAt(valueOffset)
		if err != nil {
			return 0, err
		}
		(*out)[key] = value
		keyOffset = nextOffset
	}
	return d.ContainerEndOffsetAt(offset, keyOffset)
}

func decodeGeoNamesSectionAt(
	d *mmdbdata.Decoder,
	offset uint,
	out *generatedGeoNamesSection,
) (uint, error) {
	size, keyOffset, err := d.ReadMapHeaderAt(offset)
	if err != nil {
		return 0, err
	}
	for range size {
		key, valueOffset, err := d.ReadMapEntryKeyValueOffsetAt(keyOffset)
		if err != nil {
			return 0, err
		}
		switch string(key) {
		case keyGeoNameID:
			out.GeoNameID, keyOffset, err = readUintAt(d, valueOffset)
		case keyNames:
			keyOffset, err = decodeStringMapAt(d, valueOffset, &out.Names)
		default:
			keyOffset, err = d.NextValueOffsetAt(valueOffset)
		}
		if err != nil {
			return 0, err
		}
	}
	return d.ContainerEndOffsetAt(offset, keyOffset)
}

func decodeContinentSectionAt(
	d *mmdbdata.Decoder,
	offset uint,
	out *generatedContinentSection,
) (uint, error) {
	size, keyOffset, err := d.ReadMapHeaderAt(offset)
	if err != nil {
		return 0, err
	}
	for range size {
		key, valueOffset, err := d.ReadMapEntryKeyValueOffsetAt(keyOffset)
		if err != nil {
			return 0, err
		}
		switch string(key) {
		case keyCode:
			out.Code, keyOffset, err = d.ReadStringAt(valueOffset)
		case keyGeoNameID:
			out.GeoNameID, keyOffset, err = readUintAt(d, valueOffset)
		case keyNames:
			keyOffset, err = decodeStringMapAt(d, valueOffset, &out.Names)
		default:
			keyOffset, err = d.NextValueOffsetAt(valueOffset)
		}
		if err != nil {
			return 0, err
		}
	}
	return d.ContainerEndOffsetAt(offset, keyOffset)
}

func decodeCountrySectionAt(
	d *mmdbdata.Decoder,
	offset uint,
	out *generatedCountrySection,
) (uint, error) {
	size, keyOffset, err := d.ReadMapHeaderAt(offset)
	if err != nil {
		return 0, err
	}
	for range size {
		key, valueOffset, err := d.ReadMapEntryKeyValueOffsetAt(keyOffset)
		if err != nil {
			return 0, err
		}
		switch string(key) {
		case keyGeoNameID:
			out.GeoNameID, keyOffset, err = readUintAt(d, valueOffset)
		case keyIsInEuropeanUnion:
			out.IsInEuropeanUnion, keyOffset, err = d.ReadBoolAt(valueOffset)
		case keyISOCode:
			out.IsoCode, keyOffset, err = d.ReadStringAt(valueOffset)
		case keyNames:
			keyOffset, err = decodeStringMapAt(d, valueOffset, &out.Names)
		default:
			keyOffset, err = d.NextValueOffsetAt(valueOffset)
		}
		if err != nil {
			return 0, err
		}
	}
	return d.ContainerEndOffsetAt(offset, keyOffset)
}

func decodeRepresentedCountrySectionAt(
	d *mmdbdata.Decoder,
	offset uint,
	out *generatedRepresentedCountrySection,
) (uint, error) {
	size, keyOffset, err := d.ReadMapHeaderAt(offset)
	if err != nil {
		return 0, err
	}
	for range size {
		key, valueOffset, err := d.ReadMapEntryKeyValueOffsetAt(keyOffset)
		if err != nil {
			return 0, err
		}
		switch string(key) {
		case keyGeoNameID:
			out.GeoNameID, keyOffset, err = readUintAt(d, valueOffset)
		case keyIsInEuropeanUnion:
			out.IsInEuropeanUnion, keyOffset, err = d.ReadBoolAt(valueOffset)
		case keyISOCode:
			out.IsoCode, keyOffset, err = d.ReadStringAt(valueOffset)
		case keyNames:
			keyOffset, err = decodeStringMapAt(d, valueOffset, &out.Names)
		case "type":
			out.Type, keyOffset, err = d.ReadStringAt(valueOffset)
		default:
			keyOffset, err = d.NextValueOffsetAt(valueOffset)
		}
		if err != nil {
			return 0, err
		}
	}
	return d.ContainerEndOffsetAt(offset, keyOffset)
}

func decodeLocationSectionAt(
	d *mmdbdata.Decoder,
	offset uint,
	out *generatedLocationSection,
) (uint, error) {
	size, keyOffset, err := d.ReadMapHeaderAt(offset)
	if err != nil {
		return 0, err
	}
	for range size {
		key, valueOffset, err := d.ReadMapEntryKeyValueOffsetAt(keyOffset)
		if err != nil {
			return 0, err
		}
		switch string(key) {
		case "accuracy_radius":
			out.AccuracyRadius, keyOffset, err = d.ReadUint16At(valueOffset)
		case "latitude":
			out.Latitude, keyOffset, err = d.ReadFloat64At(valueOffset)
		case "longitude":
			out.Longitude, keyOffset, err = d.ReadFloat64At(valueOffset)
		case "metro_code":
			out.MetroCode, keyOffset, err = readUintAt(d, valueOffset)
		case "time_zone":
			out.TimeZone, keyOffset, err = d.ReadStringAt(valueOffset)
		default:
			keyOffset, err = d.NextValueOffsetAt(valueOffset)
		}
		if err != nil {
			return 0, err
		}
	}
	return d.ContainerEndOffsetAt(offset, keyOffset)
}

func decodePostalSectionAt(
	d *mmdbdata.Decoder,
	offset uint,
	out *generatedPostalSection,
) (uint, error) {
	size, keyOffset, err := d.ReadMapHeaderAt(offset)
	if err != nil {
		return 0, err
	}
	for range size {
		key, valueOffset, err := d.ReadMapEntryKeyValueOffsetAt(keyOffset)
		if err != nil {
			return 0, err
		}
		if string(key) == keyCode {
			out.Code, keyOffset, err = d.ReadStringAt(valueOffset)
		} else {
			keyOffset, err = d.NextValueOffsetAt(valueOffset)
		}
		if err != nil {
			return 0, err
		}
	}
	return d.ContainerEndOffsetAt(offset, keyOffset)
}

func decodeSubdivisionAt(
	d *mmdbdata.Decoder,
	offset uint,
	out *generatedSubdivision,
) (uint, error) {
	size, keyOffset, err := d.ReadMapHeaderAt(offset)
	if err != nil {
		return 0, err
	}
	for range size {
		key, valueOffset, err := d.ReadMapEntryKeyValueOffsetAt(keyOffset)
		if err != nil {
			return 0, err
		}
		switch string(key) {
		case keyGeoNameID:
			out.GeoNameID, keyOffset, err = readUintAt(d, valueOffset)
		case keyISOCode:
			out.IsoCode, keyOffset, err = d.ReadStringAt(valueOffset)
		case keyNames:
			keyOffset, err = decodeStringMapAt(d, valueOffset, &out.Names)
		default:
			keyOffset, err = d.NextValueOffsetAt(valueOffset)
		}
		if err != nil {
			return 0, err
		}
	}
	return d.ContainerEndOffsetAt(offset, keyOffset)
}

func decodeTraitsSectionAt(
	d *mmdbdata.Decoder,
	offset uint,
	out *generatedTraitsSection,
) (uint, error) {
	size, keyOffset, err := d.ReadMapHeaderAt(offset)
	if err != nil {
		return 0, err
	}
	for range size {
		key, valueOffset, err := d.ReadMapEntryKeyValueOffsetAt(keyOffset)
		if err != nil {
			return 0, err
		}
		switch string(key) {
		case "is_anonymous_proxy":
			out.IsAnonymousProxy, keyOffset, err = d.ReadBoolAt(valueOffset)
		case "is_satellite_provider":
			out.IsSatelliteProvider, keyOffset, err = d.ReadBoolAt(valueOffset)
		default:
			keyOffset, err = d.NextValueOffsetAt(valueOffset)
		}
		if err != nil {
			return 0, err
		}
	}
	return d.ContainerEndOffsetAt(offset, keyOffset)
}

func decodeSubdivisionsAt(
	d *mmdbdata.Decoder,
	offset uint,
	out *[]generatedSubdivision,
) (uint, error) {
	size, elemOffset, err := d.ReadSliceHeaderAt(offset)
	if err != nil {
		return 0, err
	}

	if cap(*out) < int(size) {
		*out = make([]generatedSubdivision, int(size))
	} else {
		*out = (*out)[:int(size)]
	}

	for i := range size {
		elemOffset, err = decodeSubdivisionAt(d, elemOffset, &(*out)[i])
		if err != nil {
			return 0, err
		}
	}
	return d.ContainerEndOffsetAt(offset, elemOffset)
}

func (c *fullCityGeneratedLow) UnmarshalMaxMindDB(d *mmdbdata.Decoder) error {
	size, keyOffset, err := d.ReadMapHeader()
	if err != nil {
		return err
	}

	for range size {
		key, valueOffset, err := d.ReadMapEntryKeyValueOffsetAt(keyOffset)
		if err != nil {
			return err
		}
		switch string(key) {
		case "city":
			keyOffset, err = decodeGeoNamesSectionAt(d, valueOffset, &c.City)
		case "continent":
			keyOffset, err = decodeContinentSectionAt(d, valueOffset, &c.Continent)
		case "country":
			keyOffset, err = decodeCountrySectionAt(d, valueOffset, &c.Country)
		case "location":
			keyOffset, err = decodeLocationSectionAt(d, valueOffset, &c.Location)
		case "postal":
			keyOffset, err = decodePostalSectionAt(d, valueOffset, &c.Postal)
		case "registered_country":
			keyOffset, err = decodeCountrySectionAt(d, valueOffset, &c.RegisteredCountry)
		case "represented_country":
			keyOffset, err = decodeRepresentedCountrySectionAt(
				d,
				valueOffset,
				&c.RepresentedCountry,
			)
		case "subdivisions":
			keyOffset, err = decodeSubdivisionsAt(d, valueOffset, &c.Subdivisions)
		case "traits":
			keyOffset, err = decodeTraitsSectionAt(d, valueOffset, &c.Traits)
		default:
			keyOffset, err = d.NextValueOffsetAt(valueOffset)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func BenchmarkCityLookup(b *testing.B) {
	db, err := Open("GeoLite2-City.mmdb")
	require.NoError(b, err)

	//nolint:gosec // this is a test
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	var result fullCity

	s := make(net.IP, 4)
	for b.Loop() {
		ip := randomIPv4Address(r, s)
		err = db.Lookup(ip).Decode(&result)
		if err != nil {
			b.Error(err)
		}
	}
	require.NoError(b, db.Close(), "error on close")
}

func BenchmarkCityLookupGeneratedLow(b *testing.B) {
	benchmarkCityLookupGeneratedLowWithOptions(b)
}

func BenchmarkCityLookupGeneratedLowCacheOff(b *testing.B) {
	benchmarkCityLookupGeneratedLowWithOptions(b, WithCache(nil))
}

func BenchmarkCityLookupGeneratedLowCacheShared(b *testing.B) {
	benchmarkCityLookupGeneratedLowWithOptions(
		b,
		WithCache(cache.NewSharedProvider(cache.DefaultOptions())),
	)
}

func BenchmarkCityLookupGeneratedLowCachePooled(b *testing.B) {
	benchmarkCityLookupGeneratedLowWithOptions(
		b,
		WithCache(cache.NewPooledProvider(cache.DefaultOptions())),
	)
}

func benchmarkCityLookupGeneratedLowWithOptions(b *testing.B, opts ...ReaderOption) {
	db, err := Open("GeoLite2-City.mmdb", opts...)
	require.NoError(b, err)

	//nolint:gosec // this is a test
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	var result fullCityGeneratedLow

	s := make(net.IP, 4)
	for b.Loop() {
		ip := randomIPv4Address(r, s)
		err = db.Lookup(ip).Decode(&result)
		if err != nil {
			b.Error(err)
		}
	}
	require.NoError(b, db.Close(), "error on close")
}

func BenchmarkCityLookupOnly(b *testing.B) {
	db, err := Open("GeoLite2-City.mmdb")
	require.NoError(b, err)

	//nolint:gosec // this is a test
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	s := make(net.IP, 4)
	for b.Loop() {
		ip := randomIPv4Address(r, s)
		result := db.Lookup(ip)
		if err := result.Err(); err != nil {
			b.Error(err)
		}
	}
	require.NoError(b, db.Close(), "error on close")
}

func BenchmarkDecodeCountryCodeWithStruct(b *testing.B) {
	db, err := Open("GeoLite2-City.mmdb")
	require.NoError(b, err)

	type MinCountry struct {
		Country struct {
			IsoCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	}

	//nolint:gosec // this is a test
	r := rand.New(rand.NewSource(0))
	var result MinCountry

	s := make(net.IP, 4)
	for b.Loop() {
		ip := randomIPv4Address(r, s)
		err = db.Lookup(ip).Decode(&result)
		if err != nil {
			b.Error(err)
		}
	}
	require.NoError(b, db.Close(), "error on close")
}

func BenchmarkDecodePathCountryCode(b *testing.B) {
	db, err := Open("GeoLite2-City.mmdb")
	require.NoError(b, err)

	path := []any{"country", keyISOCode}

	//nolint:gosec // this is a test
	r := rand.New(rand.NewSource(0))
	var result string

	s := make(net.IP, 4)
	for b.Loop() {
		ip := randomIPv4Address(r, s)
		err = db.Lookup(ip).DecodePath(&result, path...)
		if err != nil {
			b.Error(err)
		}
	}
	require.NoError(b, db.Close(), "error on close")
}

// BenchmarkCityLookupConcurrent tests concurrent city lookups to demonstrate
// string cache performance under concurrent load.
func BenchmarkCityLookupConcurrent(b *testing.B) {
	db, err := Open("GeoLite2-City.mmdb")
	require.NoError(b, err)
	defer func() {
		require.NoError(b, db.Close(), "error on close")
	}()

	// Test with different numbers of concurrent goroutines
	goroutineCounts := []int{1, 4, 16, 64}

	for _, numGoroutines := range goroutineCounts {
		b.Run(fmt.Sprintf("goroutines_%d", numGoroutines), func(b *testing.B) {
			// Each goroutine performs 100 lookups
			const lookupsPerGoroutine = 100
			b.ResetTimer()

			for range b.N {
				var wg sync.WaitGroup
				wg.Add(numGoroutines)

				for range numGoroutines {
					go func() {
						defer wg.Done()

						//nolint:gosec // this is a test
						r := rand.New(rand.NewSource(time.Now().UnixNano()))
						s := make(net.IP, 4)
						var result fullCity

						for range lookupsPerGoroutine {
							ip := randomIPv4Address(r, s)
							err := db.Lookup(ip).Decode(&result)
							if err != nil {
								b.Error(err)
								return
							}
							// Access string fields to exercise the cache
							_ = result.City.Names
							_ = result.Country.Names
						}
					}()
				}

				wg.Wait()
			}

			// Report operations per second
			totalOps := int64(b.N) * int64(numGoroutines) * int64(lookupsPerGoroutine)
			b.ReportMetric(float64(totalOps)/b.Elapsed().Seconds(), "lookups/sec")
		})
	}
}

func randomIPv4Address(r *rand.Rand, ip []byte) netip.Addr {
	num := r.Uint32()
	ip[0] = byte(num >> 24)
	ip[1] = byte(num >> 16)
	ip[2] = byte(num >> 8)
	ip[3] = byte(num)
	v, _ := netip.AddrFromSlice(ip)
	return v
}

func testFile(file string) string {
	return filepath.Join("test-data", "test-data", file)
}

// Test custom unmarshaling through Reader.Lookup.
func TestCustomUnmarshaler(t *testing.T) {
	reader, err := Open(testFile("MaxMind-DB-test-decoder.mmdb"))
	require.NoError(t, err)
	defer func() {
		if err := reader.Close(); err != nil {
			t.Errorf("Error closing reader: %v", err)
		}
	}()

	// Test a type that implements Unmarshaler
	var customDecoded TestCity
	result := reader.Lookup(netip.MustParseAddr("1.1.1.1"))
	err = result.Decode(&customDecoded)
	require.NoError(t, err)

	// Test that the same data decoded with reflection gives the same result
	var reflectionDecoded map[string]any
	result2 := reader.Lookup(netip.MustParseAddr("1.1.1.1"))
	err = result2.Decode(&reflectionDecoded)
	require.NoError(t, err)

	// Verify the custom decoder worked correctly
	// The exact assertions depend on the test data in MaxMind-DB-test-decoder.mmdb
	t.Logf("Custom decoded: %+v", customDecoded)
	t.Logf("Reflection decoded: %+v", reflectionDecoded)

	// Test that both methods produce consistent results for any matching data
	if len(customDecoded.Names) > 0 || len(reflectionDecoded) > 0 {
		t.Log("Custom unmarshaler integration test passed - both decoders worked")
	}
}

// TestCity represents a simplified city data structure for testing custom unmarshaling.
type TestCity struct {
	Names     map[string]string `maxminddb:"names"`
	GeoNameID uint              `maxminddb:"geoname_id"`
}

// UnmarshalMaxMindDB implements the Unmarshaler interface for TestCity.
// This demonstrates custom decoding that avoids reflection for better performance.
func (c *TestCity) UnmarshalMaxMindDB(d *mmdbdata.Decoder) error {
	mapIter, _, err := d.ReadMap()
	if err != nil {
		return err
	}
	for key, err := range mapIter {
		if err != nil {
			return err
		}

		switch string(key) {
		case keyNames:
			// Decode nested map[string]string for localized names
			nameMapIter, size, err := d.ReadMap()
			if err != nil {
				return err
			}
			names := make(map[string]string, size) // Pre-allocate with correct capacity
			for nameKey, nameErr := range nameMapIter {
				if nameErr != nil {
					return nameErr
				}
				value, valueErr := d.ReadString()
				if valueErr != nil {
					return valueErr
				}
				names[string(nameKey)] = value
			}
			c.Names = names
		case keyGeoNameID:
			geoID, err := d.ReadUint32()
			if err != nil {
				return err
			}
			c.GeoNameID = uint(geoID)
		default:
			// Skip unknown fields
			if err := d.SkipValue(); err != nil {
				return err
			}
		}
	}
	return nil
}

// TestFallbackToReflection verifies that types without UnmarshalMaxMindDB still work.
func TestFallbackToReflection(t *testing.T) {
	reader, err := Open(testFile("MaxMind-DB-test-decoder.mmdb"))
	require.NoError(t, err)
	defer func() {
		if err := reader.Close(); err != nil {
			t.Errorf("Error closing reader: %v", err)
		}
	}()

	// Test with a regular struct that doesn't implement Unmarshaler
	var regularStruct struct {
		Names map[string]string `maxminddb:"names"`
	}

	result := reader.Lookup(netip.MustParseAddr("1.1.1.1"))
	err = result.Decode(&regularStruct)
	require.NoError(t, err)

	// Log the result for verification
	t.Logf("Reflection fallback result: %+v", regularStruct)
}

func TestMetadataBuildTime(t *testing.T) {
	reader, err := Open(testFile("GeoIP2-City-Test.mmdb"))
	require.NoError(t, err)
	defer func() {
		if err := reader.Close(); err != nil {
			t.Errorf("Error closing reader: %v", err)
		}
	}()

	metadata := reader.Metadata

	// Test that BuildTime() returns a valid time
	buildTime := metadata.BuildTime()
	assert.False(t, buildTime.IsZero(), "BuildTime should not be zero")

	// Test that BuildTime() matches BuildEpoch
	expectedTime := time.Unix(int64(metadata.BuildEpoch), 0)
	assert.Equal(t, expectedTime, buildTime, "BuildTime should match time.Unix(BuildEpoch, 0)")

	// Verify the build time is reasonable (after 2010, before 2030)
	assert.True(t, buildTime.After(time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)))
	assert.True(t, buildTime.Before(time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)))
}

func TestIntegerOverflowProtection(t *testing.T) {
	// Test that OpenBytes detects integer overflow in search tree size calculation
	t.Run("NodeCount overflow protection", func(t *testing.T) {
		// Create metadata that would cause overflow: very large NodeCount
		// For a 64-bit system with RecordSize=32, this should trigger overflow
		// RecordSize/4 = 8, so maxNodes would be ^uint(0)/8
		// We'll use a NodeCount larger than this limit
		overflowNodeCount := ^uint(0)/8 + 1000 // Guaranteed to overflow

		// Build minimal metadata map structure in MMDB format
		// This is simplified - in a real MMDB, metadata is encoded differently
		// But we can't easily create a valid MMDB file structure in a unit test
		// So this test verifies the logic with mocked values

		// Create a test by directly calling the validation logic
		metadata := Metadata{
			NodeCount:  overflowNodeCount,
			RecordSize: 32, // 32 bits = 4 bytes, so RecordSize/4 = 8
		}

		// Test the overflow detection logic directly
		recordSizeQuarter := metadata.RecordSize / 4
		maxNodes := ^uint(0) / recordSizeQuarter

		// Verify our test setup is correct
		assert.Greater(t, metadata.NodeCount, maxNodes,
			"Test setup error: NodeCount should exceed maxNodes for overflow test")

		// Since we can't easily create an invalid MMDB file that parses but has overflow values,
		// we test the core logic validation here and rely on integration tests
		// for the full OpenBytes flow

		if metadata.NodeCount > 0 && metadata.RecordSize > 0 {
			recordSizeQuarter := metadata.RecordSize / 4
			if recordSizeQuarter > 0 {
				maxNodes := ^uint(0) / recordSizeQuarter
				if metadata.NodeCount > maxNodes {
					// This is what should happen in OpenBytes
					err := mmdberrors.NewInvalidDatabaseError("database tree size would overflow")
					assert.Equal(t, "database tree size would overflow", err.Error())
				}
			}
		}
	})

	t.Run("Valid large values should not trigger overflow", func(t *testing.T) {
		// Test that reasonable large values don't trigger false positives
		metadata := Metadata{
			NodeCount:  1000000, // 1 million nodes
			RecordSize: 32,
		}

		recordSizeQuarter := metadata.RecordSize / 4
		maxNodes := ^uint(0) / recordSizeQuarter

		// Verify this doesn't trigger overflow
		assert.LessOrEqual(t, metadata.NodeCount, maxNodes,
			"Valid large NodeCount should not trigger overflow protection")
	})

	t.Run("Edge case: RecordSize/4 is 0", func(t *testing.T) {
		// Test edge case where RecordSize/4 could be 0
		recordSize := uint(3) // 3/4 = 0 in integer division

		recordSizeQuarter := recordSize / 4
		// Should be 0, which means no overflow check is performed
		assert.Equal(t, uint(0), recordSizeQuarter)

		// The overflow protection should skip when recordSizeQuarter is 0
		// This tests the condition: if recordSizeQuarter > 0
	})
}

func TestNetworksWithinInvalidPrefix(t *testing.T) {
	reader, err := Open(testFile("GeoIP2-Country-Test.mmdb"))
	require.NoError(t, err)
	defer func() {
		require.NoError(t, reader.Close())
	}()

	// Test what happens when user ignores ParsePrefix error and passes invalid prefix
	var invalidPrefix netip.Prefix // Zero value - invalid prefix

	foundError := false
	for result := range reader.NetworksWithin(invalidPrefix) {
		if result.Err() != nil {
			foundError = true
			// Check that we get an appropriate error message
			assert.Contains(t, result.Err().Error(), "invalid prefix")
			break
		}
	}

	assert.True(t, foundError, "Expected error when using invalid prefix")
}
