// Copyright 2017-18 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

package nvme

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func TestNVMe(t *testing.T) {
	assert := assert.New(t)

	// Test that various structs are the size they should be
	assert.Equal(uintptr(72), unsafe.Sizeof(nvmePassthruCommand{}))
	assert.Equal(uintptr(4096), unsafe.Sizeof(nvmeIdentController{}))
	assert.Equal(uintptr(4096), unsafe.Sizeof(nvmeIdentNamespace{}))
	assert.Equal(uintptr(512), unsafe.Sizeof(nvmeSMARTLog{}))

	// More tests to follow...
}
