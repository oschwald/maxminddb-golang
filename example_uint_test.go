package maxminddb_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	maxminddb "github.com/oschwald/maxminddb-golang/v2"
	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

func TestCustomCityNormalizesKindMismatch(t *testing.T) {
	city := CustomCity{}
	_, err := city.UnmarshalMaxMindDBCursor(
		mmdbdata.NewDecoder([]byte{0x40}, 0).Cursor(),
	)
	var typeError maxminddb.UnmarshalTypeError
	if !errors.As(err, &typeError) {
		t.Fatalf("expected UnmarshalTypeError, got %v", err)
	}
}

func TestCustomCityRejectsGeoNameIDOverflowOn32Bit(t *testing.T) {
	if strconv.IntSize != 32 {
		t.Skip("32-bit platform boundary")
	}

	data := []byte{
		0xe1,
		0x4a, 'g', 'e', 'o', 'n', 'a', 'm', 'e', '_', 'i', 'd',
		0x05, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00,
	}
	city := CustomCity{GeoNameID: 7}
	_, err := city.unmarshalCity(mmdbdata.NewDecoder(data, 0).Cursor())
	if err == nil || !strings.Contains(err.Error(), "cannot unmarshal") {
		t.Fatalf("expected conversion error, got %v", err)
	}
	if city.GeoNameID != 7 {
		t.Fatalf("overflow mutated GeoNameID to %d", city.GeoNameID)
	}
}
