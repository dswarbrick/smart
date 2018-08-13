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

// Package smart is a pure Go SMART library.
//
package smart

import (
	"path/filepath"

	"github.com/dswarbrick/smart/scsi"
)

// TODO: Make this discover NVMe and MegaRAID devices also.
func ScanDevices() []scsi.SCSIDevice {
	var devices []scsi.SCSIDevice

	// Find all SCSI disk devices
	files, err := filepath.Glob("/dev/sd*[^0-9]")
	if err != nil {
		return devices
	}

	for _, file := range files {
		devices = append(devices, scsi.SCSIDevice{Name: file})
	}

	return devices
}
