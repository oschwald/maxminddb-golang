// Package maxminddb provides a reader for the MaxMind DB file format.
package maxminddb

import (
	"bytes"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
)

const dataSectionSeparatorSize = 16

var metadataStartMarker = []byte("\xAB\xCD\xEFMaxMind.com")

// Reader holds the data corresponding to the MaxMind DB file. Its only public
// field is Metadata, which contains the metadata from the MaxMind DB file.
//
// All of the methods on Reader are thread-safe. The struct may be safely
// shared across goroutines.
type Reader struct {
	nodeReader        nodeReader
	buffer            []byte
	decoder           decoder
	Metadata          Metadata
	ipv4Start         uint
	ipv4StartBitDepth int
	nodeOffsetMult    uint
	hasMappedFile     bool
}

// Metadata holds the metadata decoded from the MaxMind DB file. In particular
// it has the format version, the build time as Unix epoch time, the database
// type and description, the IP version supported, and a slice of the natural
// languages included.
type Metadata struct {
	Description              map[string]string `maxminddb:"description"`
	DatabaseType             string            `maxminddb:"database_type"`
	Languages                []string          `maxminddb:"languages"`
	BinaryFormatMajorVersion uint              `maxminddb:"binary_format_major_version"`
	BinaryFormatMinorVersion uint              `maxminddb:"binary_format_minor_version"`
	BuildEpoch               uint              `maxminddb:"build_epoch"`
	IPVersion                uint              `maxminddb:"ip_version"`
	NodeCount                uint              `maxminddb:"node_count"`
	RecordSize               uint              `maxminddb:"record_size"`
}

// FromBytes takes a byte slice corresponding to a MaxMind DB file and returns
// a Reader structure or an error.
func FromBytes(buffer []byte) (*Reader, error) {
	metadataStart := bytes.LastIndex(buffer, metadataStartMarker)

	if metadataStart == -1 {
		return nil, newInvalidDatabaseError("error opening database: invalid MaxMind DB file")
	}

	metadataStart += len(metadataStartMarker)
	metadataDecoder := decoder{buffer: buffer[metadataStart:]}

	var metadata Metadata

	rvMetadata := reflect.ValueOf(&metadata)
	_, err := metadataDecoder.decode(0, rvMetadata, 0)
	if err != nil {
		return nil, err
	}

	searchTreeSize := metadata.NodeCount * metadata.RecordSize / 4
	dataSectionStart := searchTreeSize + dataSectionSeparatorSize
	dataSectionEnd := uint(metadataStart - len(metadataStartMarker))
	if dataSectionStart > dataSectionEnd {
		return nil, newInvalidDatabaseError("the MaxMind DB contains invalid metadata")
	}
	d := decoder{
		buffer: buffer[searchTreeSize+dataSectionSeparatorSize : metadataStart-len(metadataStartMarker)],
	}

	nodeBuffer := buffer[:searchTreeSize]
	var nodeReader nodeReader
	switch metadata.RecordSize {
	case 24:
		nodeReader = nodeReader24{buffer: nodeBuffer}
	case 28:
		nodeReader = nodeReader28{buffer: nodeBuffer}
	case 32:
		nodeReader = nodeReader32{buffer: nodeBuffer}
	default:
		return nil, newInvalidDatabaseError("unknown record size: %d", metadata.RecordSize)
	}

	reader := &Reader{
		buffer:         buffer,
		nodeReader:     nodeReader,
		decoder:        d,
		Metadata:       metadata,
		ipv4Start:      0,
		nodeOffsetMult: metadata.RecordSize / 4,
	}

	reader.setIPv4Start()

	return reader, err
}

func (r *Reader) setIPv4Start() {
	if r.Metadata.IPVersion != 6 {
		r.ipv4StartBitDepth = 96
		return
	}

	nodeCount := r.Metadata.NodeCount

	node := uint(0)
	i := 0
	for ; i < 96 && node < nodeCount; i++ {
		node = r.nodeReader.readLeft(node * r.nodeOffsetMult)
	}
	r.ipv4Start = node
	r.ipv4StartBitDepth = i
}

// Lookup retrieves the database record for ip and returns Result, which can
// be used to decode the data..
func (r *Reader) Lookup(ip netip.Addr) Result {
	if r.buffer == nil {
		return Result{err: errors.New("cannot call Lookup on a closed database")}
	}
	pointer, prefixLen, err := r.lookupPointer(ip)
	if err != nil {
		return Result{
			ip:        ip,
			prefixLen: uint8(prefixLen),
			err:       err,
		}
	}
	if pointer == 0 {
		return Result{
			ip:        ip,
			prefixLen: uint8(prefixLen),
			offset:    notFound,
		}
	}
	offset, err := r.resolveDataPointer(pointer)
	return Result{
		decoder:   r.decoder,
		ip:        ip,
		offset:    uint(offset),
		prefixLen: uint8(prefixLen),
		err:       err,
	}
}

// Decode the record at |offset| into |result|. The result value pointed to
// must be a data value that corresponds to a record in the database. This may
// include a struct representation of the data, a map capable of holding the
// data or an empty any value.
//
// If result is a pointer to a struct, the struct need not include a field
// for every value that may be in the database. If a field is not present in
// the structure, the decoder will not decode that field, reducing the time
// required to decode the record.
//
// As a special case, a struct field of type uintptr will be used to capture
// the offset of the value. Decode may later be used to extract the stored
// value from the offset. MaxMind DBs are highly normalized: for example in
// the City database, all records of the same country will reference a
// single representative record for that country. This uintptr behavior allows
// clients to leverage this normalization in their own sub-record caching.
func (r *Reader) Decode(offset uintptr, result any) error {
	if r.buffer == nil {
		return errors.New("cannot call Decode on a closed database")
	}

	return Result{decoder: r.decoder, offset: uint(offset)}.Decode(result)
}

var zeroIP = netip.MustParseAddr("::")

func (r *Reader) lookupPointer(ip netip.Addr) (uint, int, error) {
	if r.Metadata.IPVersion == 4 && ip.Is6() {
		return 0, 0, fmt.Errorf(
			"error looking up '%s': you attempted to look up an IPv6 address in an IPv4-only database",
			ip.String(),
		)
	}

	node, prefixLength := r.traverseTree(ip, 0, 128)

	nodeCount := r.Metadata.NodeCount
	if node == nodeCount {
		// Record is empty
		return 0, prefixLength, nil
	} else if node > nodeCount {
		return node, prefixLength, nil
	}

	return 0, prefixLength, newInvalidDatabaseError("invalid node in search tree")
}

func (r *Reader) traverseTree(ip netip.Addr, node uint, stopBit int) (uint, int) {
	i := 0
	if ip.Is4() {
		i = r.ipv4StartBitDepth
		node = r.ipv4Start
	}
	nodeCount := r.Metadata.NodeCount

	ip16 := ip.As16()

	for ; i < stopBit && node < nodeCount; i++ {
		bit := uint(1) & (uint(ip16[i>>3]) >> (7 - (i % 8)))

		offset := node * r.nodeOffsetMult
		if bit == 0 {
			node = r.nodeReader.readLeft(offset)
		} else {
			node = r.nodeReader.readRight(offset)
		}
	}

	return node, i
}

func (r *Reader) retrieveData(pointer uint, result any) error {
	offset, err := r.resolveDataPointer(pointer)
	if err != nil {
		return err
	}
	return Result{decoder: r.decoder, offset: uint(offset)}.Decode(result)
}

func (r *Reader) resolveDataPointer(pointer uint) (uintptr, error) {
	resolved := uintptr(pointer - r.Metadata.NodeCount - dataSectionSeparatorSize)

	if resolved >= uintptr(len(r.buffer)) {
		return 0, newInvalidDatabaseError("the MaxMind DB file's search tree is corrupt")
	}
	return resolved, nil
}
