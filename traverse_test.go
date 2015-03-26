package maxminddb

import (
	"fmt"
	"testing"
)

func TestTraverse(t *testing.T) {
	for _, recordSize := range []uint{24, 28, 32} {
		for _, ipVersion := range []uint{4, 6} {
			fileName := fmt.Sprintf("test-data/test-data/MaxMind-DB-test-ipv%d-%d.mmdb", ipVersion, recordSize)
			reader, err := Open(fileName)
			if err != nil {
				t.Fatalf("unexpected error while opening database: %v", err)
			}
			defer reader.Close()

			i := reader.Traverse()

			for {
				var recordInterface interface{}
				network, err := i.Next(&recordInterface)
				if err != nil {
					t.Fatal(err)
				} else if network == nil {
					break
				}

				record := recordInterface.(map[string]interface{})

				if record["ip"] != network.IP.String() {
					t.Fatalf("expected %s got %s", record["ip"], network.IP.String())
				}
			}
		}
	}
}
