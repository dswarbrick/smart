// Copyright 2017 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

// Miscellaneous bit operations.

package smart

import (
	"fmt"
	"math/bits"
)

// log2b finds the most significant bit set in a uint.
func log2b(x uint) int {
	return bits.Len(x) - 1
}

// swapBytes swaps the order of every second byte in a byte slice (modifies slice in-place).
func swapBytes(s []byte) []byte {
	for i := 0; i < len(s); i += 2 {
		s[i], s[i+1] = s[i+1], s[i]
	}

	return s
}

// formatBytes formats a uint64 byte quantity using human-readble units, e.g. kilobyte, megabyte.
// TODO: Add big.Int variant of this function.
func formatBytes(v uint64) string {
	var i int

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
