package maxminddb

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/mitchellh/mapstructure"
	"net"
	"os"
	"syscall"
)

const dataSectionSeparatorSize = 16

var metadataStartMarker = []byte("\xAB\xCD\xEFMaxMind.com")

type Reader struct {
	file      *os.File
	buffer    []byte
	decoder   decoder
	Metadata  Metadata
	ipv4Start uint
}

type Metadata struct {
	BinaryFormatMajorVersion uint   `maxminddb:"binary_format_major_version"`
	BinaryFormatMinorVersion uint   `maxminddb:"binary_format_minor_version"`
	BuildEpoch               uint   `maxminddb:"build_epoch"`
	DatabaseType             string `maxminddb:"database_type"`
	Description              map[string]string
	IPVersion                uint `maxminddb:"ip_version"`
	Languages                []string
	NodeCount                uint `maxminddb:"node_count"`
	RecordSize               uint `maxminddb:"record_size"`
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

	reader, err := OpenBytes(mmap)
	if err != nil {
		syscall.Munmap(mmap)
		mapFile.Close()
		return nil, err
	}

	reader.file = mapFile
	return reader, nil
}

func OpenBytes(mmap []byte) (*Reader, error) {
	metadataStart := bytes.LastIndex(mmap, metadataStartMarker)

	if metadataStart == -1 {
		return nil, fmt.Errorf("error opening database file: invalid MaxMind DB file")
	}

	metadataStart += len(metadataStartMarker)
	metadataDecoder := decoder{mmap, uint(metadataStart)}

	metadataInterface, _, err := metadataDecoder.decode(uint(metadataStart))
	if err != nil {
		return nil, err
	}
	var metadata Metadata
	config := &mapstructure.DecoderConfig{
		TagName: "maxminddb",
		Result:  &metadata,
	}
	metadataUnmarshaler, err := mapstructure.NewDecoder(config)
	if err != nil {
		return nil, err
	}

	err = metadataUnmarshaler.Decode(metadataInterface)
	if err != nil {
		return nil, err
	}

	searchTreeSize := metadata.NodeCount * metadata.RecordSize / 4
	decoder := decoder{mmap, searchTreeSize + dataSectionSeparatorSize}

	return &Reader{buffer: mmap, decoder: decoder, Metadata: metadata, ipv4Start: 0}, nil
}

func (r *Reader) Lookup(ipAddress net.IP) (interface{}, error) {
	if len(ipAddress) == 16 && r.Metadata.IPVersion == 4 {
		return nil, fmt.Errorf("error looking up '%s': you attempted to look up an IPv6 address in an IPv4-only database", ipAddress.String())
	}

	pointer, err := r.findAddressInTree(ipAddress)

	if pointer == 0 {
		return nil, err
	}
	return r.resolveDataPointer(pointer)
}

func (r *Reader) Unmarshal(ipAddress net.IP, result interface{}) error {
	config := &mapstructure.DecoderConfig{
		TagName: "maxminddb",
		Result:  result,
	}
	mapDecoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return err
	}

	rawValue, err := r.Lookup(ipAddress)
	if err != nil {
		return err
	}
	return mapDecoder.Decode(rawValue)
}

func (r *Reader) findAddressInTree(ipAddress net.IP) (uint, error) {
	bitCount := uint(len(ipAddress) * 8)
	node, err := r.startNode(bitCount)
	if err != nil {
		return 0, err
	}
	NodeCount := r.Metadata.NodeCount

	for i := uint(0); i < bitCount && node < NodeCount; i++ {
		bit := uint(1) & (uint(ipAddress[i>>3]) >> (7 - (i % 8)))

		var err error
		node, err = r.readNode(node, bit)
		if err != nil {
			return 0, err
		}
	}
	if node == NodeCount {
		// Record is empty
		return 0, nil
	} else if node > NodeCount {
		return node, nil
	}

	return 0, errors.New("invalid node in search tree")
}

func (r *Reader) startNode(length uint) (uint, error) {
	if r.Metadata.IPVersion != 6 || length == 128 {
		return 0, nil
	}

	// We are looking up an IPv4 address in an IPv6 tree. Skip over the
	// first 96 nodes.
	if r.ipv4Start != 0 {
		return r.ipv4Start, nil
	}
	NodeCount := r.Metadata.NodeCount

	node := uint(0)
	for i := 0; i < 96 && node < NodeCount; i++ {
	}
	var err error
	node, err = r.readNode(node, 0)
	r.ipv4Start = node
	return node, err
}

func (r *Reader) readNode(nodeNumber uint, index uint) (uint, error) {
	RecordSize := r.Metadata.RecordSize

	baseOffset := nodeNumber * RecordSize / 4

	var nodeBytes []byte
	switch RecordSize {
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
		return 0, fmt.Errorf("unknown record size: %d", RecordSize)
	}
	return uintFromBytes(nodeBytes), nil
}

func (r *Reader) resolveDataPointer(pointer uint) (interface{}, error) {
	NodeCount := r.Metadata.NodeCount
	searchTreeSize := r.Metadata.RecordSize * NodeCount / 4

	resolved := pointer - NodeCount + searchTreeSize

	if resolved > uint(len(r.buffer)) {
		return nil, errors.New("the MaxMind DB file's search tree is corrupt")
	}

	data, _, err := r.decoder.decode(resolved)
	return data, err
}

func (r *Reader) Close() {
	if r.file != nil {
		syscall.Munmap(r.buffer)
		r.file.Close()
	}
}
