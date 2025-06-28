package maxminddb

import (
	"runtime"

	"github.com/oschwald/maxminddb-golang/v2/internal/mmdberrors"
)

type verifier struct {
	reader *Reader
}

// Verify checks that the database is valid. It validates the search tree,
// the data section, and the metadata section. This verifier is stricter than
// the specification and may return errors on databases that are readable.
func (r *Reader) Verify() error {
	v := verifier{r}
	if err := v.verifyMetadata(); err != nil {
		return err
	}

	err := v.verifyDatabase()
	runtime.KeepAlive(v.reader)
	return err
}

func (v *verifier) verifyMetadata() error {
	metadata := v.reader.Metadata

	if metadata.BinaryFormatMajorVersion != 2 {
		return testError(
			"binary_format_major_version",
			2,
			metadata.BinaryFormatMajorVersion,
		)
	}

	if metadata.BinaryFormatMinorVersion != 0 {
		return testError(
			"binary_format_minor_version",
			0,
			metadata.BinaryFormatMinorVersion,
		)
	}

	if metadata.DatabaseType == "" {
		return testError(
			"database_type",
			"non-empty string",
			metadata.DatabaseType,
		)
	}

	if len(metadata.Description) == 0 {
		return testError(
			"description",
			"non-empty slice",
			metadata.Description,
		)
	}

	if metadata.IPVersion != 4 && metadata.IPVersion != 6 {
		return testError(
			"ip_version",
			"4 or 6",
			metadata.IPVersion,
		)
	}

	if metadata.RecordSize != 24 &&
		metadata.RecordSize != 28 &&
		metadata.RecordSize != 32 {
		return testError(
			"record_size",
			"24, 28, or 32",
			metadata.RecordSize,
		)
	}

	if metadata.NodeCount == 0 {
		return testError(
			"node_count",
			"positive integer",
			metadata.NodeCount,
		)
	}
	return nil
}

func (v *verifier) verifyDatabase() error {
	offsets, err := v.verifySearchTree()
	if err != nil {
		return err
	}

	if err := v.verifyDataSectionSeparator(); err != nil {
		return err
	}

	return v.reader.decoder.VerifyDataSection(offsets)
}

func (v *verifier) verifySearchTree() (map[uint]bool, error) {
	offsets := make(map[uint]bool)

	for result := range v.reader.Networks() {
		if err := result.Err(); err != nil {
			return nil, err
		}
		offsets[result.offset] = true
	}
	return offsets, nil
}

func (v *verifier) verifyDataSectionSeparator() error {
	separatorStart := v.reader.Metadata.NodeCount * v.reader.Metadata.RecordSize / 4

	separator := v.reader.buffer[separatorStart : separatorStart+dataSectionSeparatorSize]

	for _, b := range separator {
		if b != 0 {
			return mmdberrors.NewInvalidDatabaseError(
				"unexpected byte in data separator: %v",
				separator,
			)
		}
	}
	return nil
}

func testError(
	field string,
	expected any,
	actual any,
) error {
	return mmdberrors.NewInvalidDatabaseError(
		"%v - Expected: %v Actual: %v",
		field,
		expected,
		actual,
	)
}
