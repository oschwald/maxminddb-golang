package fixture

import "github.com/oschwald/maxminddb-golang/v2/mmdbdata"

type BenchmarkCustomKey string

func (key *BenchmarkCustomKey) UnmarshalMaxMindDBCursor(
	cursor mmdbdata.Cursor,
) (mmdbdata.Cursor, error) {
	value, next, err := cursor.ReadString()
	if err == nil {
		*key = BenchmarkCustomKey(value)
	}
	return next, err
}

type BenchmarkCustomKeyRecord struct {
	Values map[BenchmarkCustomKey]bool `maxminddb:"values"`
}
