package maxminddb

import (
	"bytes"
	"encoding/hex"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2/internal/decoder"
)

// FuzzDatabase tests MMDB file parsing and IP address lookups.
// This targets file format parsing, database initialization, and lookup operations.
func FuzzDatabase(f *testing.F) {
	// Add all test MMDB files as seeds
	for _, filename := range getAllTestMMDBFiles() {
		if seedData, err := os.ReadFile(testFile(filename)); err == nil {
			f.Add(seedData)
		}
	}

	// Add malformed data patterns
	f.Add([]byte("not an mmdb file"))
	f.Add([]byte{0x00, 0x01, 0x02, 0x03})
	f.Add(bytes.Repeat([]byte{0xFF}, 1024))
	f.Add([]byte{})

	f.Fuzz(func(_ *testing.T, data []byte) {
		reader, err := OpenBytes(data)
		if err != nil {
			return
		}
		defer func() { _ = reader.Close() }()

		// Test IP lookup and data decoding
		result := reader.Lookup(netip.MustParseAddr("1.1.1.1"))
		if result.Err() == nil {
			var mapResult map[string]any
			_ = result.Decode(&mapResult)
			if mapResult != nil {
				var output any
				_ = result.DecodePath(&output, "country", "iso_code")
			}
		}
	})
}

// FuzzLookup tests IP address lookups without decoding results.
// This isolates the tree traversal and lookup logic from data decoding.
func FuzzLookup(f *testing.F) {
	// Add test MMDB files as seeds to fuzz the databases
	for _, filename := range getAllTestMMDBFiles() {
		if seedData, err := os.ReadFile(testFile(filename)); err == nil {
			f.Add(seedData)
		}
	}

	// Add malformed database patterns
	f.Add([]byte("not an mmdb file"))
	f.Add([]byte{0x00, 0x01, 0x02, 0x03})
	f.Add(bytes.Repeat([]byte{0xFF}, 512))
	f.Add([]byte{})

	// Fixed test IP addresses to use for lookups
	testIPs := []netip.Addr{
		netip.MustParseAddr("1.1.1.1"),
		netip.MustParseAddr("216.160.83.56"), // Known test IP with data
		netip.MustParseAddr("2.125.160.216"), // Another known test IP
		netip.MustParseAddr("::1"),           // IPv6
		netip.MustParseAddr("2001:218::"),    // IPv6 with data
	}

	f.Fuzz(func(_ *testing.T, data []byte) {
		reader, err := OpenBytes(data)
		if err != nil {
			return
		}
		defer func() { _ = reader.Close() }()

		if reader.Metadata.DatabaseType == "" {
			return
		}

		// Test lookups with fixed IPs - focus on tree traversal logic
		for _, addr := range testIPs {
			result := reader.Lookup(addr)

			// Check that we get a valid result (error or not)
			// Don't decode the data, just verify the lookup completed
			_ = result.Err()

			// Also test that we can get basic result properties without decoding
			_ = result.Found()
		}
	})
}

// FuzzDecodePath tests path-based decoding with fuzzed path segments.
// This targets edge cases in path traversal logic.
func FuzzDecodePath(f *testing.F) {
	// Use a complex test database with nested structures
	testDB := testFile("GeoIP2-City-Test.mmdb")
	reader, err := Open(testDB)
	if err != nil {
		f.Skip("Could not open test database")
		return
	}
	defer func() { _ = reader.Close() }()

	// Use a known IP that has complex data
	result := reader.Lookup(netip.MustParseAddr("2.125.160.216"))
	if result.Err() != nil {
		f.Skip("Could not perform lookup")
		return
	}

	// Add seed paths based on known data structure
	seedPaths := [][]string{
		{"country", "iso_code"},
		{"city", "names", "en"},
		{"location", "latitude"},
		{"location", "longitude"},
		{"postal", "code"},
		{"subdivisions", "0", "iso_code"},
		{"continent", "code"},
		{"registered_country", "iso_code"},
		{"country", "names", "en"},
		{"city", "geoname_id"},
		{"subdivisions", "0", "names", "en"},
		{"traits", "is_anonymous_proxy"},
		{"location", "accuracy_radius"},
		{"location", "metro_code"},
		{"location", "time_zone"},
	}

	for _, path := range seedPaths {
		// Encode path as bytes with null separators
		pathBytes := make([]byte, 0)
		for i, segment := range path {
			if i > 0 {
				pathBytes = append(pathBytes, 0) // null separator
			}
			pathBytes = append(pathBytes, []byte(segment)...)
		}
		f.Add(pathBytes)
	}

	// Add some edge case seeds
	f.Add([]byte(""))                      // empty path
	f.Add([]byte("nonexistent"))           // single segment
	f.Add(bytes.Repeat([]byte("a"), 1000)) // very long segment
	f.Add([]byte("key\x00with\x00nulls"))  // embedded nulls
	f.Add([]byte("123\x00456\x00789"))     // numeric-looking paths
	f.Add([]byte("utf8\x00spëçîål"))       // unicode characters

	f.Fuzz(func(_ *testing.T, pathData []byte) {
		// Skip completely empty data
		if len(pathData) == 0 {
			return
		}

		// Parse path data into segments using null byte separators
		segments := bytes.Split(pathData, []byte{0})
		if len(segments) == 0 {
			return
		}

		// Convert byte segments to path elements
		var path []any
		for _, segment := range segments {
			// Skip empty segments
			if len(segment) == 0 {
				continue
			}

			segmentStr := string(segment)
			// Try to convert numeric strings to integers for array indexing
			if num, isInt := parseSimpleInt(segmentStr); isInt {
				path = append(path, num)
			} else {
				path = append(path, segmentStr)
			}
		}

		// Skip if we ended up with no path elements
		if len(path) == 0 {
			return
		}

		// Try to decode with the fuzzed path
		var output any
		_ = result.DecodePath(&output, path...)

		// Also test with different output types to exercise different decoding paths
		var stringOutput string
		_ = result.DecodePath(&stringOutput, path...)

		var intOutput int
		_ = result.DecodePath(&intOutput, path...)

		var mapOutput map[string]any
		_ = result.DecodePath(&mapOutput, path...)

		var sliceOutput []any
		_ = result.DecodePath(&sliceOutput, path...)
	})
}

// FuzzNetworks tests the Networks() iterator with malformed databases.
// This focuses specifically on tree traversal and iteration logic.
func FuzzNetworks(f *testing.F) {
	// Add test MMDB files as seeds
	for _, filename := range getAllTestMMDBFiles() {
		if seedData, err := os.ReadFile(testFile(filename)); err == nil {
			f.Add(seedData)
		}
	}

	// Add malformed data patterns
	f.Add([]byte("not an mmdb file"))
	f.Add([]byte{0x00, 0x01, 0x02, 0x03})
	f.Add(bytes.Repeat([]byte{0xFF}, 512))

	f.Fuzz(func(_ *testing.T, data []byte) {
		reader, err := OpenBytes(data)
		if err != nil {
			return
		}
		defer func() { _ = reader.Close() }()

		if reader.Metadata.DatabaseType == "" {
			return
		}

		// Test Networks() iteration with conservative limits
		count := 0
		for result := range reader.Networks() {
			if result.Err() != nil || count >= 5 {
				break
			}
			count++
			var output any
			_ = result.Decode(&output)
		}
	})
}

// FuzzDecode tests the ReflectionDecoder.Decode method with fuzzed data.
// This targets data section parsing and reflection-based decoding logic.
func FuzzDecode(f *testing.F) {
	// Add raw test data file as seed
	if rawData, err := os.ReadFile(testFile("maps-with-pointers.raw")); err == nil {
		f.Add(rawData)
	}

	// Add validated test data from decoder tests
	testHexStrings := []string{
		// Float64 values
		"680000000000000000", // 0.0
		"683FE0000000000000", // 0.5
		"68400921FB54442EEA", // 3.14159265359
		"68405EC00000000000", // 123.0
		"6841D000000007F8F4", // 1073741824.12457
		"68BFE0000000000000", // -0.5
		"68C00921FB54442EEA", // -3.14159265359
		"68C1D000000007F8F4", // -1073741824.12457

		// Float32 values
		"040800000000", // 0.0
		"04083F800000", // 1.0
		"04083F8CCCCD", // 1.1
		"04084048F5C3", // 3.14
		"0408461C3FF6", // 9999.99
		"0408BF800000", // -1.0
		"0408BF8CCCCD", // -1.1
		"0408C048F5C3", // -3.14
		"0408C61C3FF6", // -9999.99

		// Integer values
		"0401ffffffff", // -1
		"0401ffffff01", // -255
		"020101f4",     // 500

		// Boolean values
		"0007", // false
		"0107", // true

		// Maps
		"E0",                             // Empty map
		"e142656e43466f6f",               // {"en": "Foo"}
		"e242656e43466f6f427a6843e4baba", // {"en": "Foo", "zh": "人"}
		"e1446e616d65e242656e43466f6f427a6843e4baba", // Nested map
		"e1496c616e677561676573020442656e427a68",     // Map with array value

		// Arrays
		"020442656e427a68", // ["en", "zh"]

		// Strings
		"43466f6f", // "Foo"
		"42656e",   // "en"
		"427a68",   // "zh"
	}

	for _, hexStr := range testHexStrings {
		if data, err := hex.DecodeString(hexStr); err == nil {
			f.Add(data)
		}
	}

	// Add malformed data patterns
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	f.Add([]byte{0x42, 0x48, 0x65, 0x6C, 0x6C, 0x6F})
	f.Add([]byte{0x60, 0x41, 0x61, 0x41, 0x62})
	f.Add([]byte{0xE1, 0x41, 0x61, 0x41, 0x62})

	f.Fuzz(func(_ *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}

		reflectionDecoder := decoder.New(data)

		// Test decoding into various types
		outputs := []any{
			new(map[string]any),
			new(string),
			new(int),
			new(uint32),
			new(float64),
			new(bool),
			new([]any),
			new([]string),
			new(map[string]string),
			new([]map[string]any),
			new(any),
		}

		for _, output := range outputs {
			_ = reflectionDecoder.Decode(0, output)
		}

		// Test different offsets
		for offset := uint(1); offset < uint(len(data)) && offset < 10; offset++ {
			var mapOutput map[string]any
			_ = reflectionDecoder.Decode(offset, &mapOutput)
		}
	})
}

// parseSimpleInt converts numeric strings to integers with bounds checking.
// Returns the integer and true if valid, or 0 and false if not a simple integer.
func parseSimpleInt(s string) (int, bool) {
	num, err := strconv.Atoi(s)
	if err != nil || num < -1000 || num > 1000 {
		return 0, false
	}
	return num, true
}

// getAllTestMMDBFiles returns smaller MMDB files from the test-data directory.
// Large files are excluded to keep fuzzing fast and prevent timeouts.
func getAllTestMMDBFiles() []string {
	testDataDir := filepath.Join("test-data", "test-data")
	entries, err := os.ReadDir(testDataDir)
	if err != nil {
		return nil
	}

	var mmdbFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".mmdb" {
			// Check file size - skip very large files for fuzzing performance
			if info, err := entry.Info(); err == nil && info.Size() < 5000 { // 5KB limit
				mmdbFiles = append(mmdbFiles, entry.Name())
			}
		}
	}
	return mmdbFiles
}
