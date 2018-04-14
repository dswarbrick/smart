// Copyright 2017-18 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

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
