// Copyright 2017-18 Daniel Swarbrick. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
