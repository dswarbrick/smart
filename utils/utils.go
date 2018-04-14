// Copyright 2017-18 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

// Miscellaneous utility functions

package utils

import (
	"encoding/binary"
	"math/bits"
	"unsafe"
)

var (
	NativeEndian binary.ByteOrder
)

// Determine native endianness of system
func init() {
	i := uint32(1)
	b := (*[4]byte)(unsafe.Pointer(&i))
	if b[0] == 1 {
		NativeEndian = binary.LittleEndian
	} else {
		NativeEndian = binary.BigEndian
	}
}

// log2b finds the most significant bit set in a uint.
func Log2b(x uint) int {
	if x == 0 {
		return 0
	}

	return bits.Len(x) - 1
}

// SwapBytes swaps the order of every second byte in a byte slice (modifies slice in-place).
func SwapBytes(s []byte) []byte {
	for i := 0; i < len(s); i += 2 {
		s[i], s[i+1] = s[i+1], s[i]
	}

	return s
}
