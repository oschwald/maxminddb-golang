package maxminddb

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResultPrefixUsesIPv4StartBitDepth(t *testing.T) {
	reader := &Reader{
		Metadata:          Metadata{IPVersion: 6, NodeCount: 8},
		ipv4Start:         4,
		ipv4StartBitDepth: 64,
	}

	result := Result{
		reader:    reader,
		ip:        netip.MustParseAddr("1.2.3.4"),
		prefixLen: 88,
	}

	assert.Equal(t, "1.2.3.0/24", result.Prefix().String())
}

func TestResultPrefixReturnsIPv6NetworkWithoutIPv4Subtree(t *testing.T) {
	reader := &Reader{
		Metadata:          Metadata{IPVersion: 6, NodeCount: 8},
		ipv4Start:         8,
		ipv4StartBitDepth: 64,
	}

	result := Result{
		reader:    reader,
		ip:        netip.MustParseAddr("1.2.3.4"),
		prefixLen: 64,
	}

	assert.Equal(t, "::/64", result.Prefix().String())
}
