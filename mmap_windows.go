//go:build windows && !appengine
// +build windows,!appengine

package maxminddb

// Windows support largely borrowed from mmap-go.
//
// Copyright (c) 2011, Evan Shaw <edsrzf@gmail.com>
// All rights reserved.

// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//     * Redistributions of source code must retain the above copyright
//       notice, this list of conditions and the following disclaimer.
//     * Redistributions in binary form must reproduce the above copyright
//       notice, this list of conditions and the following disclaimer in the
//       documentation and/or other materials provided with the distribution.
//     * Neither the name of the copyright holder nor the
//       names of its contributors may be used to endorse or promote products
//       derived from this software without specific prior written permission.

// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL <COPYRIGHT HOLDER> BE LIABLE FOR ANY
// DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
// (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
// LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
// ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
// SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

import (
	"errors"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows
var (
	handleLock sync.Mutex
	handleMap  = map[uintptr]windows.Handle{}
)

// mmap maps a file into memory and returns a byte slice.
func mmap(fd int, length int) ([]byte, error) {
	// Create a file mapping
	handle, err := windows.CreateFileMapping(
		windows.Handle(fd),
		nil,
		windows.PAGE_READONLY,
		0,
		uint32(length),
		nil,
	)
	if err != nil {
		return nil, os.NewSyscallError("CreateFileMapping", err)
	}

	// Map the file into memory
	addr, err := windows.MapViewOfFile(
		handle,
		windows.FILE_MAP_READ,
		0,
		0,
		uintptr(length),
	)
	if err != nil {
		windows.CloseHandle(handle)
		return nil, os.NewSyscallError("MapViewOfFile", err)
	}

	// Store the handle in the map
	handleLock.Lock()
	handleMap[addr] = handle
	handleLock.Unlock()

	data := unsafe.Slice((*byte)(unsafe.Pointer(addr)), length)
	return data, nil
}

// flush ensures changes to a memory-mapped region are written to disk.
func flush(addr, length uintptr) error {
	err := windows.FlushViewOfFile(addr, length)
	if err != nil {
		return os.NewSyscallError("FlushViewOfFile", err)
	}
	return nil
}

// munmap unmaps a memory-mapped file and releases associated resources.
func munmap(b []byte) error {
	// Convert slice to base address and length
	data := unsafe.SliceData(b)
	addr := uintptr(unsafe.Pointer(data))
	length := uintptr(len(b))

	// Flush the memory region
	if err := flush(addr, length); err != nil {
		return err
	}

	// Unmap the memory
	if err := windows.UnmapViewOfFile(addr); err != nil {
		return os.NewSyscallError("UnmapViewOfFile", err)
	}

	// Remove the handle from the map and close it
	handleLock.Lock()
	defer handleLock.Unlock()

	handle, ok := handleMap[addr]
	if !ok {
		// should be impossible; we would've errored above
		return errors.New("unknown base address")
	}
	delete(handleMap, addr)

	if err := windows.CloseHandle(handle); err != nil {
		return os.NewSyscallError("CloseHandle", err)
	}
	return nil
}
