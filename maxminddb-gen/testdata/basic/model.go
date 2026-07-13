package fixture

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

type Label string

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

type Record struct {
	Name        Label             `maxminddb:"name"`
	Count       uint8             `maxminddb:"count"`
	Temperature float32           `maxminddb:"temperature"`
	Enabled     *bool             `maxminddb:"enabled"`
	Values      []uint16          `maxminddb:"values"`
	Lookup      map[string]string `maxminddb:"lookup"`
	Nested      Nested            `maxminddb:"nested"`
	Custom      Custom            `maxminddb:"custom"`
	Bytes       []byte            `maxminddb:"bytes"`
	Ignored     chan int          `maxminddb:"-"`
}
