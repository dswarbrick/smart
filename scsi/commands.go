// Copyright 2017-18 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

// SCSI command definitions.

package scsi

const (
	// SCSI commands used by this package
	SCSI_INQUIRY          = 0x12
	SCSI_MODE_SENSE_6     = 0x1a
	SCSI_READ_CAPACITY_10 = 0x25
	SCSI_ATA_PASSTHRU_16  = 0x85

	// Minimum length of standard INQUIRY response
	INQ_REPLY_LEN = 36

	// SCSI-3 mode pages
	RIGID_DISK_DRIVE_GEOMETRY_PAGE = 0x04

	// Mode page control field
	MPAGE_CONTROL_DEFAULT = 2
)

// SCSI CDB types
type CDB6 [6]byte
type CDB10 [10]byte
type CDB16 [16]byte
