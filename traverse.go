package maxminddb

import (
	"errors"
	"net"
	"reflect"
)

// Internal structure used to keep track of nodes we still need to visit.
type nodeip struct {
	node uint
	ip   net.IP
	bit  uint
}

type Iterator struct {
	r *Reader
	n []nodeip // Nodes we still have to visit.
	k uint // Which direction to go in the tree.
}

// Traverse can be used to traverse all entries in DB file.
func (r *Reader) Traverse() *Iterator {
	s := 4
	if r.Metadata.IPVersion == 6 {
		s = 16
	}

	return &Iterator{
		r: r,
		n: []nodeip{
			nodeip{
				ip: make(net.IP, s),
			},
		},
	}
}

// Next returns the next ip in the iterator. result will be filled similar to Lookup.
// The end of the iterator is reached when the return value is (nil, nil).
func (i *Iterator) Next(result interface{}) (net.IP, error) {
	rv := reflect.ValueOf(result)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return nil, errors.New("result param for Next must be a pointer")
	}

	for len(i.n) > 0 {
		node := i.n[0]

		// Have we visited both children?
		for i.k < 2 {
			pointer, err := i.r.readNode(node.node, i.k)
			if err != nil {
				return nil, err
			}

			// We need to make a copy so we don't modify the original.
			ip := make(net.IP, len(node.ip))
			copy(ip, node.ip)

			if i.k == 1 {
				ip[node.bit>>3] |= 1 << uint(7-(node.bit%8))
			}

			i.k++

			if pointer < i.r.Metadata.NodeCount {
				i.n = append(i.n, nodeip{
					node: pointer,
					ip:   ip,
					bit:  node.bit + 1,
				})
			} else if pointer > i.r.Metadata.NodeCount {
				if err = i.r.resolveDataPointer(pointer, rv); err != nil {
					return nil, err
				}

				return ip, nil
			} // pointer == i.r.Metadata.NodeCount means an empty node we can ignore.
		}

		i.n = i.n[1:]
		i.k = 0
	}

	return nil, nil
}
