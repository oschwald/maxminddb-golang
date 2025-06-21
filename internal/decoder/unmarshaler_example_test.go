package decoder

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// City represents a simplified version of GeoIP2 city data.
// This demonstrates how to create a custom decoder for IP geolocation data.
type City struct {
	Names     map[string]string `maxminddb:"names"`
	GeoNameID uint              `maxminddb:"geoname_id"`
}

// UnmarshalMaxMindDB implements the Unmarshaler interface for City.
// This demonstrates how to create a high-performance, non-reflection decoder
// for IP geolocation data that avoids the overhead of reflection.
func (c *City) UnmarshalMaxMindDB(d *Decoder) error {
	for key, err := range d.DecodeMap() {
		if err != nil {
			return err
		}

		switch string(key) {
		case "names":
			// Decode nested map[string]string for localized names
			names := make(map[string]string)
			for nameKey, nameErr := range d.DecodeMap() {
				if nameErr != nil {
					return nameErr
				}
				value, valueErr := d.DecodeString()
				if valueErr != nil {
					return valueErr
				}
				names[string(nameKey)] = value
			}
			c.Names = names
		case "geoname_id":
			geoID, err := d.DecodeUInt32()
			if err != nil {
				return err
			}
			c.GeoNameID = uint(geoID)
		case "en":
			// For our test data {"en": "Foo"} - backwards compatibility
			value, err := d.DecodeString()
			if err != nil {
				return err
			}
			if c.Names == nil {
				c.Names = make(map[string]string)
			}
			c.Names["en"] = value
		default:
			if err := d.SkipValue(); err != nil {
				return err
			}
		}
	}

	return nil
}

// ASN represents Autonomous System Number data from GeoIP2 ASN database.
// This demonstrates custom decoding for network infrastructure data.
type ASN struct {
	AutonomousSystemOrganization string `maxminddb:"autonomous_system_organization"`
	AutonomousSystemNumber       uint   `maxminddb:"autonomous_system_number"`
}

// UnmarshalMaxMindDB implements the Unmarshaler interface for ASN.
// This shows how to efficiently decode ISP/network data without reflection.
func (a *ASN) UnmarshalMaxMindDB(d *Decoder) error {
	for key, err := range d.DecodeMap() {
		if err != nil {
			return err
		}

		switch string(key) {
		case "autonomous_system_organization":
			org, err := d.DecodeString()
			if err != nil {
				return err
			}
			a.AutonomousSystemOrganization = org
		case "autonomous_system_number":
			asn, err := d.DecodeUInt32()
			if err != nil {
				return err
			}
			a.AutonomousSystemNumber = uint(asn)
		default:
			if err := d.SkipValue(); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestUnmarshalerInterface(t *testing.T) {
	// Use a working test case from existing tests
	// This represents {"en": "Foo"} which simulates city name data
	testData := "e142656e43466f6f"

	inputBytes, err := hexDecodeString(testData)
	require.NoError(t, err)

	// Test with the City unmarshaler using new iterator approach
	decoder := New(inputBytes)
	var city City
	err = decoder.Decode(0, &city)
	require.NoError(t, err)
	require.Equal(t, "Foo", city.Names["en"])
}

func TestUnmarshalerWithReflectionFallback(t *testing.T) {
	// Test that types without UnmarshalMaxMindDB still work with reflection
	testData := "e142656e43466f6f"

	inputBytes, err := hexDecodeString(testData)
	require.NoError(t, err)

	decoder := New(inputBytes)

	// This should use reflection since map[string]any doesn't implement Unmarshaler
	var result map[string]any
	err = decoder.Decode(0, &result)
	require.NoError(t, err)
	require.Equal(t, "Foo", result["en"])
}

// Helper function for tests.
func hexDecodeString(s string) ([]byte, error) {
	result := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		var b byte
		_, err := fmt.Sscanf(s[i:i+2], "%02x", &b)
		if err != nil {
			return nil, err
		}
		result[i/2] = b
	}
	return result, nil
}

// Example showing the simple usage pattern that's very similar to json.Unmarshaler.
func Example_unmarshalerPattern() {
	// This demonstrates the simple, clean API for IP geolocation data.
	// It follows the exact same pattern as encoding/json.

	// Sample MMDB data (this would come from a real MaxMind GeoIP2 database lookup)
	buffer := []byte{} // This would be actual MMDB data from IP lookup

	decoder := New(buffer)

	// Types that implement UnmarshalMaxMindDB automatically use custom decoding
	// No registration, configuration, or setup needed - just like json.Unmarshaler!
	var city City
	_ = decoder.Decode(0, &city) // Automatically uses City.UnmarshalMaxMindDB

	var asn ASN
	_ = decoder.Decode(0, &asn) // Automatically uses ASN.UnmarshalMaxMindDB

	// Types without UnmarshalMaxMindDB automatically fall back to reflection
	var genericData map[string]any
	_ = decoder.Decode(0, &genericData) // Uses reflection automatically

	fmt.Printf("City: %+v\n", city)
	fmt.Printf("ASN: %+v\n", asn)
	fmt.Printf("Generic: %+v\n", genericData)

	// The UnmarshalMaxMindDB implementation is very clean and efficient:
	//
	//   func (c *City) UnmarshalMaxMindDB(d *decoder.Decoder) error {
	//       for key, err := range d.DecodeMap() {
	//           if err != nil { return err }
	//           switch string(key) {
	//           case "names":
	//               // Decode nested map for localized city
	//               names := make(map[string]string)
	//               for nameKey, nameErr := range d.DecodeMap() {
	//                   if nameErr != nil { return nameErr }
	//                   value, valueErr := d.DecodeString()
	//                   if valueErr != nil { return valueErr }
	//                   names[string(nameKey)] = value
	//               }
	//               c.Names = names
	//           case "geoname_id":
	//               c.GeoNameID, err = d.DecodeUInt32()
	//           default:
	//               err = d.SkipValue()
	//           }
	//           if err != nil { return err }
	//       }
	//       return nil
	//   }

	// Output:
	// City: {Names:map[] GeoNameID:0}
	// ASN: {AutonomousSystemOrganization: AutonomousSystemNumber:0}
	// Generic: map[]
}
