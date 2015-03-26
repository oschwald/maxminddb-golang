package maxminddb

import (
	"errors"
	"net"
	"reflect"
)

// Internal structure used to keep track of nodes we still need to visit.
type nodeip struct {
	pointer uint
	ip      net.IP
	bit     uint
}

type Iterator struct {
	reader *Reader
	nodes  []nodeip // Nodes we still have to visit.
}

// Traverse can be used to traverse all entries in DB file.
func (r *Reader) Traverse() *Iterator {
	s := 4
	if r.Metadata.IPVersion == 6 {
		s = 16
	}
	return &Iterator{
		reader: r,
		nodes: []nodeip{
			nodeip{
				ip: make(net.IP, s),
			},
		},
	}
}

// Next returns the next ip in the iterator. result will be filled similar to Lookup.
// The end of the iterator is reached when the return value is (nil, nil).
func (it *Iterator) Next(result interface{}) (*net.IPNet, error) {
	rv := reflect.ValueOf(result)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return nil, errors.New("result param for Next must be a pointer")
	}

	for len(it.nodes) > 0 {
		node := it.nodes[len(it.nodes)-1]
		it.nodes = it.nodes[:len(it.nodes)-1]

		for {
			if node.pointer < it.reader.Metadata.NodeCount {
				ipRight := make(net.IP, len(node.ip))
				copy(ipRight, node.ip)
				ipRight[node.bit>>3] |= 1 << uint(7-(node.bit%8))

				rightPointer, err := it.reader.readNode(node.pointer, 1)
				if err != nil {
					return nil, err
				}

				node.bit++
				it.nodes = append(it.nodes, nodeip{
					pointer: rightPointer,
					ip:      ipRight,
					bit:     node.bit,
				})

				node.pointer, err = it.reader.readNode(node.pointer, 0)
				if err != nil {
					return nil, err
				}

			} else if node.pointer > it.reader.Metadata.NodeCount {
				if err := it.reader.resolveDataPointer(node.pointer, rv); err != nil {
					return nil, err
				}

				return &net.IPNet{
					IP:   node.ip,
					Mask: net.CIDRMask(int(node.bit), len(node.ip)*8),
				}, nil
			} else {
				break
			}
		}
	}

	return nil, nil
}
