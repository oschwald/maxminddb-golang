package maxminddb

import (
	"fmt"
	"net"
)

// Internal structure used to keep track of nodes we still need to visit.
type netNode struct {
	ip      net.IP
	bit     uint
	pointer uint
}

// Networks represents a set of subnets that we are iterating over.
type Networks struct {
	reader   *Reader
	nodes    []netNode // Nodes we still have to visit.
	lastNode netNode
	subnet   *net.IPNet
	supernet *netNode
	err      error
}

// Networks returns an iterator that can be used to traverse all networks in
// the database.
//
// Please note that a MaxMind DB may map IPv4 networks into several locations
// in in an IPv6 database. This iterator will iterate over all of these
// locations separately.
func (r *Reader) Networks() *Networks {
	s := 4
	if r.Metadata.IPVersion == 6 {
		s = 16
	}
	return &Networks{
		reader: r,
		nodes: []netNode{
			{
				ip: make(net.IP, s),
			},
		},
	}
}

// NetworksWithin returns an iterator that can be used to traverse all networks
// in the database which are contained in a given network.
//
// Please note that a MaxMind DB may map IPv4 networks into several locations
// in in an IPv6 database. This iterator will iterate over all of these
// locations separately.

func (r *Reader) NetworksWithin(network net.IPNet) *Networks {
	s := 4
	if r.Metadata.IPVersion == 6 {
		s = 16
	}
	return &Networks{
		reader: r,
		nodes: []netNode{
			{
				ip: make(net.IP, s),
			},
		},
		subnet: &network,
	}
}

// Next prepares the next network for reading with the Network method. It
// returns true if there is another network to be processed and false if there
// are no more networks or if there is an error.
func (n *Networks) Next() bool {
	for len(n.nodes) > 0 {
		node := n.nodes[len(n.nodes)-1]
		n.nodes = n.nodes[:len(n.nodes)-1]

		for node.pointer != n.reader.Metadata.NodeCount {
			if node.pointer > n.reader.Metadata.NodeCount {
				n.lastNode = node
				return true
			}
			ipRight := make(net.IP, len(node.ip))
			copy(ipRight, node.ip)
			if len(ipRight) <= int(node.bit>>3) {
				n.err = newInvalidDatabaseError(
					"invalid search tree at %v/%v", ipRight, node.bit)
				return false
			}
			ipRight[node.bit>>3] |= 1 << (7 - (node.bit % 8))

			offset := node.pointer * n.reader.nodeOffsetMult
			rightPointer := n.reader.nodeReader.readRight(offset)

			node.bit++
			n.nodes = append(n.nodes, netNode{
				pointer: rightPointer,
				ip:      ipRight,
				bit:     node.bit,
			})

			node.pointer = n.reader.nodeReader.readLeft(offset)
		}
	}

	return false
}

func (n *Networks) NextNetworkWithin() bool {
	if n.subnet == nil {
		fmt.Println("No network to search within")
		return false
	}

	for n.Next() {
		if n.subnet.Contains(n.lastNode.ip) {
			return true
		}

		if n.subnet != nil {
			maybeSupernet := net.IPNet{
				IP:   n.lastNode.ip,
				Mask: net.CIDRMask(int(n.lastNode.bit), len(n.lastNode.ip)*8),
			}
			if maybeSupernet.Contains(n.subnet.IP) {
				n.supernet = &n.lastNode
			}
		}
	}
	return false
}

// Network returns the current network or an error if there is a problem
// decoding the data for the network. It takes a pointer to a result value to
// decode the network's data into.
func (n *Networks) Network(result interface{}) (*net.IPNet, error) {
	if err := n.reader.retrieveData(n.lastNode.pointer, result); err != nil {
		return nil, err
	}

	return &net.IPNet{
		IP:   n.lastNode.ip,
		Mask: net.CIDRMask(int(n.lastNode.bit), len(n.lastNode.ip)*8),
	}, nil
}

// Supernet returns nil if no NetworksWithin search has taken place or if no
// supernet has been found.  It returns the last found supernet or an error if
// there is a problem decoding the data for the network. It takes a pointer to
// a result value to decode the network's data into.

func (n *Networks) Supernet(result interface{}) (*net.IPNet, error) {
	if n.supernet == nil {
		return nil, nil
	}

	if err := n.reader.retrieveData(n.supernet.pointer, result); err != nil {
		return nil, err
	}

	return &net.IPNet{
		IP:   n.supernet.ip,
		Mask: net.CIDRMask(int(n.supernet.bit), len(n.supernet.ip)*8),
	}, nil
}

func (n *Networks) NodesRemaining() int {
	return len(n.nodes)
}

// Err returns an error, if any, that was encountered during iteration.
func (n *Networks) Err() error {
	return n.err
}
