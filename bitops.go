// Copyright 2017 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.
//
// Portions Copyright 2017 The Go Authors. All rights reserved.

// Miscellaneous bit operations.

package smart

func bitLen(x uint) (n uint) {
	for ; x >= 0x8000; x >>= 16 {
		n += 16
	}
	if x >= 0x80 {
		x >>= 8
		n += 8
	}
	if x >= 0x8 {
		x >>= 4
		n += 4
	}
	if x >= 0x2 {
		x >>= 2
		n += 2
	}
	if x >= 0x1 {
		n++
	}
	return
}

// log2b finds the most significant bit set in a uint.
func log2b(x uint) uint {
	return bitLen(x) - 1
}

// swapBytes swaps the order of every second byte in a byte slice (modifies slice in-place).
func swapBytes(s []byte) []byte {
	for i := 0; i < len(s); i += 2 {
		s[i], s[i+1] = s[i+1], s[i]
	}

	return s
}
