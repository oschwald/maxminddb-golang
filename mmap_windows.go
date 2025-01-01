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
	"reflect"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type memoryMap []byte

// Windows
var handleLock sync.Mutex
var handleMap = map[uintptr]windows.Handle{}

func mmap(fd int, length int) (data []byte, err error) {
	h, errno := windows.CreateFileMapping(windows.Handle(fd), nil,
		uint32(windows.PAGE_READONLY), 0, uint32(length), nil)
	if h == 0 {
		return nil, os.NewSyscallError("CreateFileMapping", errno)
	}

	addr, errno := windows.MapViewOfFile(h, uint32(windows.FILE_MAP_READ), 0,
		0, uintptr(length))
	if addr == 0 {
		return nil, os.NewSyscallError("MapViewOfFile", errno)
	}
	handleLock.Lock()
	handleMap[addr] = h
	handleLock.Unlock()

	m := memoryMap{}
	dh := m.header()
	dh.Data = addr
	dh.Len = length
	dh.Cap = dh.Len

	return m, nil
}

func (m *memoryMap) header() *reflect.SliceHeader {
	return (*reflect.SliceHeader)(unsafe.Pointer(m))
}

func flush(addr, len uintptr) error {
	errno := windows.FlushViewOfFile(addr, len)
	return os.NewSyscallError("FlushViewOfFile", errno)
}

func munmap(b []byte) (err error) {
	m := memoryMap(b)
	dh := m.header()

	addr := dh.Data
	length := uintptr(dh.Len)

	flush(addr, length)
	err = windows.UnmapViewOfFile(addr)
	if err != nil {
		return err
	}

	handleLock.Lock()
	defer handleLock.Unlock()
	handle, ok := handleMap[addr]
	if !ok {
		// should be impossible; we would've errored above
		return errors.New("unknown base address")
	}
	delete(handleMap, addr)

	e := windows.CloseHandle(windows.Handle(handle))
	return os.NewSyscallError("CloseHandle", e)
}
