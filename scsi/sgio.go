// Copyright 2017-18 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

// SCSI generic IO functions.

package scsi

import (
	"fmt"
)

const (
	SG_DXFER_NONE        = -1
	SG_DXFER_TO_DEV      = -2
	SG_DXFER_FROM_DEV    = -3
	SG_DXFER_TO_FROM_DEV = -4

	SG_INFO_OK_MASK = 0x1
	SG_INFO_OK      = 0x0

	SG_IO = 0x2285

	// Timeout in milliseconds
	DEFAULT_TIMEOUT = 20000
)

type SgioError struct {
	ScsiStatus   uint8
	HostStatus   uint16
	DriverStatus uint16
	senseBuf     [32]byte // FIXME: This is not yet populated by anything
}

func (e SgioError) Error() string {
	return fmt.Sprintf("SCSI status: %#02x, host status: %#02x, driver status: %#02x",
		e.ScsiStatus, e.HostStatus, e.DriverStatus)
}
