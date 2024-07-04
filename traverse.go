package maxminddb

import (
	"fmt"
	"net/netip"
)

// Internal structure used to keep track of nodes we still need to visit.
type netNode struct {
	ip      netip.Addr
	bit     uint
	pointer uint
}

// Networks represents a set of subnets that we are iterating over.
type Networks struct {
	err                    error
	reader                 *Reader
	nodes                  []netNode
	lastNode               netNode
	includeAliasedNetworks bool
}

var (
	allIPv4 = netip.MustParsePrefix("0.0.0.0/0")
	allIPv6 = netip.MustParsePrefix("::/0")
)

// NetworksOption are options for Networks and NetworksWithin.
type NetworksOption func(*Networks)

// IncludeAliasedNetworks is an option for Networks and NetworksWithin
// that makes them iterate over aliases of the IPv4 subtree in an IPv6
// database, e.g., ::ffff:0:0/96, 2001::/32, and 2002::/16.
func IncludeAliasedNetworks(networks *Networks) {
	networks.includeAliasedNetworks = true
}

// Networks returns an iterator that can be used to traverse all networks in
// the database.
//
// Please note that a MaxMind DB may map IPv4 networks into several locations
// in an IPv6 database. This iterator will only iterate over these once by
// default. To iterate over all the IPv4 network locations, use the
// IncludeAliasedNetworks option.
func (r *Reader) Networks(options ...NetworksOption) *Networks {
	var networks *Networks
	if r.Metadata.IPVersion == 6 {
		networks = r.NetworksWithin(allIPv6, options...)
	} else {
		networks = r.NetworksWithin(allIPv4, options...)
	}

	return networks
}

// NetworksWithin returns an iterator that can be used to traverse all networks
// in the database which are contained in a given prefix.
//
// Please note that a MaxMind DB may map IPv4 networks into several locations
// in an IPv6 database. This iterator will iterate over all of these locations
// separately. To only iterate over the IPv4 networks once, use the
// SkipAliasedNetworks option.
//
// If the provided prefix is contained within a network in the database, the
// iterator will iterate over exactly one network, the containing network.
func (r *Reader) NetworksWithin(prefix netip.Prefix, options ...NetworksOption) *Networks {
	if r.Metadata.IPVersion == 4 && prefix.Addr().Is6() {
		return &Networks{
			err: fmt.Errorf(
				"error getting networks with '%s': you attempted to use an IPv6 network in an IPv4-only database",
				prefix,
			),
		}
	}

	networks := &Networks{reader: r}
	for _, option := range options {
		option(networks)
	}

	ip := prefix.Addr()
	netIP := ip
	stopBit := prefix.Bits()
	if ip.Is4() {
		netIP = v4ToV16(ip)
		stopBit += 96
	}

	pointer, bit := r.traverseTree(ip, 0, stopBit)

	prefix, err := netIP.Prefix(bit)
	if err != nil {
		networks.err = fmt.Errorf("prefixing %s with %d", netIP, bit)
	}

	networks.nodes = []netNode{
		{
			ip:      prefix.Addr(),
			bit:     uint(bit),
			pointer: pointer,
		},
	}

	return networks
}

// Next prepares the next network for reading with the Network method. It
// returns true if there is another network to be processed and false if there
// are no more networks or if there is an error.
func (n *Networks) Next() bool {
	if n.err != nil {
		return false
	}
	for len(n.nodes) > 0 {
		node := n.nodes[len(n.nodes)-1]
		n.nodes = n.nodes[:len(n.nodes)-1]

		for node.pointer != n.reader.Metadata.NodeCount {
			// This skips IPv4 aliases without hardcoding the networks that the writer
			// currently aliases.
			if !n.includeAliasedNetworks && n.reader.ipv4Start != 0 &&
				node.pointer == n.reader.ipv4Start && !isInIPv4Subtree(node.ip) {
				break
			}

			if node.pointer > n.reader.Metadata.NodeCount {
				n.lastNode = node
				return true
			}
			ipRight := node.ip.As16()
			if len(ipRight) <= int(node.bit>>3) {
				displayAddr := node.ip
				displayBits := node.bit
				if isInIPv4Subtree(node.ip) {
					displayAddr = v6ToV4(displayAddr)
					displayBits -= 96
				}

				n.err = newInvalidDatabaseError(
					"invalid search tree at %s/%d", displayAddr, displayBits)
				return false
			}
			ipRight[node.bit>>3] |= 1 << (7 - (node.bit % 8))

			offset := node.pointer * n.reader.nodeOffsetMult
			rightPointer := n.reader.nodeReader.readRight(offset)

			node.bit++
			n.nodes = append(n.nodes, netNode{
				pointer: rightPointer,
				ip:      netip.AddrFrom16(ipRight),
				bit:     node.bit,
			})

			node.pointer = n.reader.nodeReader.readLeft(offset)
		}
	}

	return false
}

// Network returns the current network or an error if there is a problem
// decoding the data for the network. It takes a pointer to a result value to
// decode the network's data into.
func (n *Networks) Network(result any) (netip.Prefix, error) {
	if n.err != nil {
		return netip.Prefix{}, n.err
	}
	if err := n.reader.retrieveData(n.lastNode.pointer, result); err != nil {
		return netip.Prefix{}, err
	}

	ip := n.lastNode.ip
	prefixLength := int(n.lastNode.bit)
	if isInIPv4Subtree(ip) {
		ip = v6ToV4(ip)
		prefixLength -= 96
	}

	return netip.PrefixFrom(ip, prefixLength), nil
}

// Err returns an error, if any, that was encountered during iteration.
func (n *Networks) Err() error {
	return n.err
}

var ipv4SubtreeBoundary = netip.MustParseAddr("::255.255.255.255").Next()

// isInIPv4Subtree returns true if the IP is in the database's IPv4 subtree.
func isInIPv4Subtree(ip netip.Addr) bool {
	return ip.Is4() || ip.Less(ipv4SubtreeBoundary)
}

// We store IPv4 addresses at ::/96 for unclear reasons.
func v4ToV16(ip netip.Addr) netip.Addr {
	b4 := ip.As4()
	var b16 [16]byte
	copy(b16[12:], b4[:])
	return netip.AddrFrom16(b16)
}

// Converts an IPv4 address embedded in IPv6 to IPv4.
func v6ToV4(ip netip.Addr) netip.Addr {
	b := ip.As16()
	v, _ := netip.AddrFromSlice(b[12:])
	return v
}
