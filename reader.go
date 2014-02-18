package maxminddb

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
)

const dataSectionSeparatorSize = 16

var metadataStartMarker []byte = []byte("\xAB\xCD\xEFMaxMind.com")

type Reader struct {
	file      *os.File
	buffer    []byte
	decoder   decoder
	metadata  map[string]interface{}
	ipv4Start uint
}

func Open(file string) (*Reader, error) {
	mapFile, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	stats, err := mapFile.Stat()
	if err != nil {
		return nil, err
	}

	fileSize := int(stats.Size())
	mmap, err := syscall.Mmap(int(mapFile.Fd()), 0, fileSize, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, err

	}

	metadataStart := bytes.LastIndex(mmap, metadataStartMarker)

	if metadataStart == -1 {
		syscall.Munmap(mmap)
		mapFile.Close()
		errStr := fmt.Sprintf("Error opening database file (%s). Is this a valid MaxMind DB file?", file)
		return nil, errors.New(errStr)
	}

	metadataStart += len(metadataStartMarker)
	metadataDecoder := decoder{mmap, uint(metadataStart)}

	metadataInterface, _ := metadataDecoder.decode(uint(metadataStart))
	metadata := metadataInterface.(map[string]interface{})

	searchTreeSize := metadata["node_count"].(uint) * metadata["record_size"].(uint) / 4
	decoder := decoder{mmap, searchTreeSize + dataSectionSeparatorSize}

	return &Reader{mapFile, mmap, decoder, metadata, 0}, nil
}

func (r *Reader) Lookup(ipAddress net.IP) (interface{}, error) {
	if len(ipAddress) == 16 && r.metadata["ip_version"].(uint) == 4 {
		return nil, errors.New(fmt.Sprintf("Error looking up %s. You attempted to look up an IPv6 address in an IPv4-only database.", ipAddress.String()))
	}

	pointer, err := r.findAddressInTree(ipAddress)

	if pointer == 0 {
		return nil, err
	}
	return r.resolveDataPointer(pointer), nil
}

func (r *Reader) findAddressInTree(ipAddress net.IP) (uint, error) {
	bitCount := uint(len(ipAddress) * 8)
	node := r.startNode(bitCount)
	nodeCount := r.metadata["node_count"].(uint)

	for i := uint(0); i < bitCount && node < nodeCount; i++ {
		bit := uint(1) & (uint(ipAddress[i>>3]) >> (7 - (i % 8)))

		node = r.readNode(node, bit)
	}
	if node == nodeCount {
		// Record is empty
		return 0, nil
	} else if node > nodeCount {
		return node, nil
	}

	return 0, errors.New("Invalid node in search tree")
}

func (r *Reader) startNode(length uint) uint {
	if r.metadata["ip_version"].(uint) != 6 || length == 128 {
		return 0
	}

	// We are looking up an IPv4 address in an IPv6 tree. Skip over the
	// first 96 nodes.
	if r.ipv4Start != 0 {
		return r.ipv4Start
	}
	nodeCount := r.metadata["node_count"].(uint)

	node := uint(0)
	for i := 0; i < 96 && node < nodeCount; i++ {
	}
	node = r.readNode(node, 0)
	r.ipv4Start = node
	return node
}

func (r *Reader) readNode(nodeNumber uint, index uint) uint {
	recordSize := r.metadata["record_size"].(uint)

	baseOffset := nodeNumber * recordSize / 4

	var nodeBytes []byte
	switch recordSize {
	case 24:
		offset := baseOffset + index*3
		nodeBytes = r.buffer[offset : offset+3]
	case 28:
		middle := r.buffer[baseOffset+3]
		if index != 0 {
			middle &= 0x0F
		} else {
			middle = (0xF0 & middle) >> 4
		}
		offset := baseOffset + index*4
		nodeBytes = append([]byte{middle}, r.buffer[offset:offset+3]...)
	case 32:
		offset := baseOffset + index*4
		nodeBytes = r.buffer[offset : offset+4]
	default:
		panic(fmt.Sprintf("Unknown record size: %d", recordSize))
	}
	return uintFromBytes(nodeBytes)
}

func (r *Reader) resolveDataPointer(pointer uint) interface{} {
	nodeCount := r.metadata["node_count"].(uint)
	searchTreeSize := r.metadata["record_size"].(uint) * nodeCount / 4

	resolved := pointer - nodeCount + searchTreeSize

	if resolved > uint(len(r.buffer)) {
		panic(
			"The MaxMind DB file's search tree is corrupt")
	}

	data, _ := r.decoder.decode(resolved)
	return data
}

func (r *Reader) Close() {
	syscall.Munmap(r.buffer)
	r.file.Close()
}
