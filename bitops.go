// Copyright 2017-18 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

// Miscellaneous bit operations.

package smart

import (
	"fmt"
	"math/big"
)

func formatBigBytes(v *big.Int) string {
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
