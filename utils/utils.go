// Copyright 2017-18 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

// Miscellaneous utility functions

package utils

import (
	"encoding/binary"
	"fmt"
	"math/big"
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

func FormatBigBytes(v *big.Int) string {
	var i int

	suffixes := [...]string{"B", "KB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"}
	d := big.NewInt(1)

	for i = 0; i < len(suffixes)-1; i++ {
		if v.Cmp(new(big.Int).Mul(d, big.NewInt(1000))) == 1 {
			d.Mul(d, big.NewInt(1000))
		} else {
			break
		}
	}

	if i == 0 {
		return fmt.Sprintf("%d %s", v, suffixes[i])
	} else {
		// TODO: Implement 3 significant digit printing as per formatBytes()
		return fmt.Sprintf("%d %s", v.Div(v, d), suffixes[i])
	}
}

// formatBytes formats a uint64 byte quantity using human-readble units, e.g. kilobyte, megabyte.
// TODO: Add big.Int variant of this function.
func FormatBytes(v uint64) string {
	var i int

	// Only populate to exabyte, since we are constrained by uint64 limit
	suffixes := [...]string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}
	d := uint64(1)

	for i = 0; i < len(suffixes)-1; i++ {
		if v >= d*1000 {
			d *= 1000
		} else {
			break
		}
	}

	if i == 0 {
		return fmt.Sprintf("%d %s", v, suffixes[i])
	} else {
		// Print 3 significant digits
		return fmt.Sprintf("%.3g %s", float64(v)/float64(d), suffixes[i])
	}
}

// log2b finds the most significant bit set in a uint.
func Log2b(x uint) int {
	if x == 0 {
		return 0
	}

	return bits.Len(x) - 1
}
