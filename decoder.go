package maxminddb

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

// Decoder provides methods for decoding MaxMind DB data values.
// This interface is passed to UnmarshalMaxMindDB methods to allow
// custom decoding logic that avoids reflection for performance-critical applications.
//
// Types implementing Unmarshaler will automatically use custom decoding logic
// instead of reflection when used with Reader.Lookup, providing better performance
// for performance-critical applications.
//
// Example:
//
//	type City struct {
//		Names     map[string]string `maxminddb:"names"`
//		GeoNameID uint              `maxminddb:"geoname_id"`
//	}
//
//	func (c *City) UnmarshalMaxMindDB(d *maxminddb.Decoder) error {
//		for key, err := range d.DecodeMap() {
//			if err != nil { return err }
//			switch string(key) {
//			case "names":
//				names := make(map[string]string)
//				for nameKey, nameErr := range d.DecodeMap() {
//					if nameErr != nil { return nameErr }
//					value, valueErr := d.DecodeString()
//					if valueErr != nil { return valueErr }
//					names[string(nameKey)] = value
//				}
//				c.Names = names
//			case "geoname_id":
//				geoID, err := d.DecodeUInt32()
//				if err != nil { return err }
//				c.GeoNameID = uint(geoID)
//			default:
//				if err := d.SkipValue(); err != nil { return err }
//			}
//		}
//		return nil
//	}
type Decoder = mmdbdata.Decoder

// Unmarshaler is implemented by types that can unmarshal MaxMind DB data.
// This follows the same pattern as json.Unmarshaler and other Go standard library interfaces.
type Unmarshaler = mmdbdata.Unmarshaler
