// Copyright 2017-18 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

// SCSI generic IO functions.

package smart

import (
	"github.com/dswarbrick/smart/drivedb"
	"github.com/dswarbrick/smart/scsi"
)

// Top-level device interface. All supported device types must implement these methods.
type Device interface {
	Open() error
	Close() error
	PrintSMART(*drivedb.DriveDb) error
}

func OpenSCSIAutodetect(name string) (Device, error) {
	dev := scsi.SCSIDevice{Name: name}

	if err := dev.Open(); err != nil {
		return nil, err
	}

	inquiry, err := dev.Inquiry()
	if err != nil {
		return nil, err
	}

	// Check if device is an ATA device.
	// TODO: Handle USB-SATA bridges by probing the device with an ATA IDENTIFY command. Watch out
	// for ATAPI devices.
	if inquiry.VendorIdent == [8]byte{0x41, 0x54, 0x41, 0x20, 0x20, 0x20, 0x20, 0x20} {
		return &SATDevice{dev}, nil
	}

	return &dev, nil
}
