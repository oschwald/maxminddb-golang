package fixture

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

type Label string

type Code string

type Custom string

func (c *Custom) UnmarshalMaxMindDB(decoder *mmdbdata.Decoder) error {
	value, err := decoder.ReadString()
	if err == nil {
		*c = Custom(value)
	}
	return err
}

type Nested struct {
	Name string `maxminddb:"name"`
}

type dualCustom struct {
	value       string
	cursorCalls uint8
	legacyCalls uint8
}

func (value *dualCustom) UnmarshalMaxMindDBCursor(
	cursor mmdbdata.Cursor,
) (mmdbdata.Cursor, error) {
	decoded, next, err := cursor.ReadString()
	value.cursorCalls++
	if err == nil {
		value.value = "cursor:" + decoded
	}
	return next, err
}

func (value *dualCustom) UnmarshalMaxMindDB(decoder *mmdbdata.Decoder) error {
	decoded, err := decoder.ReadString()
	value.legacyCalls++
	if err == nil {
		value.value = "legacy:" + decoded
	}
	return err
}

type Float64Record struct {
	Value float64 `maxminddb:"value"`
}

type ByteShapes struct {
	Pointer *[]byte           `maxminddb:"pointer"`
	Slices  [][]byte          `maxminddb:"slices"`
	Lookup  map[string][]byte `maxminddb:"lookup"`
}

type DualInterfaceRecord struct {
	Value dualCustom `maxminddb:"value"`
}

type Record struct {
	Name        Label             `maxminddb:"name"`
	Count       uint8             `maxminddb:"count"`
	Count32     uint32            `maxminddb:"count32"`
	Count64     uint64            `maxminddb:"count64"`
	Temperature float32           `maxminddb:"temperature"`
	Enabled     *bool             `maxminddb:"enabled"`
	Values      []uint16          `maxminddb:"values"`
	Lookup      map[string]string `maxminddb:"lookup"`
	Names       map[Code]uint8    `maxminddb:"names"`
	Nested      Nested            `maxminddb:"nested"`
	Custom      Custom            `maxminddb:"custom"`
	Bytes       []byte            `maxminddb:"bytes"`
	Ignored     chan int          `maxminddb:"-"`
}
