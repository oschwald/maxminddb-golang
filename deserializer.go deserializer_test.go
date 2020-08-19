package maxminddb

import (
	"math/big"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodingToDeserializer(t *testing.T) {
	reader, err := Open(testFile("MaxMind-DB-test-decoder.mmdb"))
	require.NoError(t, err, "unexpected error while opening database: %v", err)

	dser := testDeserializer{}
	err = reader.Lookup(net.ParseIP("::1.1.1.0"), &dser)
	require.NoError(t, err, "unexpected error while doing lookup: %v", err)

	checkDecodingToInterface(t, dser.rv)
}

type stackValue struct {
	value  interface{}
	curNum int
}

type testDeserializer struct {
	stack []*stackValue
	rv    interface{}
	key   *string
}

func (d *testDeserializer) ShouldSkip(offset uintptr) (bool, error) {
	return false, nil
}

func (d *testDeserializer) StartSlice(size uint) error {
	return d.add(make([]interface{}, size))
}

func (d *testDeserializer) StartMap(size uint) error {
	return d.add(map[string]interface{}{})
}

func (d *testDeserializer) End() error {
	d.stack = d.stack[:len(d.stack)-1]
	return nil
}

func (d *testDeserializer) String(v string) error {
	return d.add(v)
}

func (d *testDeserializer) Float64(v float64) error {
	return d.add(v)
}

func (d *testDeserializer) Bytes(v []byte) error {
	return d.add(v)
}

func (d *testDeserializer) Uint16(v uint16) error {
	return d.add(uint64(v))
}

func (d *testDeserializer) Uint32(v uint32) error {
	return d.add(uint64(v))
}

func (d *testDeserializer) Int32(v int32) error {
	return d.add(int(v))
}

func (d *testDeserializer) Uint64(v uint64) error {
	return d.add(v)
}

func (d *testDeserializer) Uint128(v *big.Int) error {
	return d.add(v)
}

func (d *testDeserializer) Bool(v bool) error {
	return d.add(v)
}

func (d *testDeserializer) Float32(v float32) error {
	return d.add(v)
}

func (d *testDeserializer) add(v interface{}) error {
	if len(d.stack) == 0 {
		d.rv = v
	} else {
		top := d.stack[len(d.stack)-1]
		switch parent := top.value.(type) {
		case map[string]interface{}:
			if d.key == nil {
				key := v.(string)
				d.key = &key
			} else {
				parent[*d.key] = v
				d.key = nil
			}

		case []interface{}:
			parent[top.curNum] = v
			top.curNum++
		default:
		}
	}

	switch v := v.(type) {
	case map[string]interface{}, []interface{}:
		d.stack = append(d.stack, &stackValue{value: v})
	default:
	}

	return nil
}
