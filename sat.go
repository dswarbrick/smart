/*
 * Pure Go SMART library
 * Copyright 2017 Daniel Swarbrick
 *
 * SCSI / ATA Translation functions
 */

package smart

const ATA_IDENTIFY_DEVICE = 0xec

// ATA device identify struct
type IdentifyDeviceData struct {
	GeneralConfiguration uint16
	NumCylinders         uint16
	ReservedWord2        uint16
	NumHeads             uint16
	Retired1             [2]uint16
	NumSectorsPerTrack   uint16
	VendorUnique         [3]uint16
	SerialNumber         [20]byte
	Retired2             [2]uint16
	Obsolete1            uint16
	FirmwareRevision     [8]byte
	ModelNumber          [40]byte
	MaxBlockTransfer     uint8
	VendorUnique2        uint8
	ReservedWord48       uint16
	Capabilities         uint32
	ObsoleteWords51      [2]uint16
	_                    [512 - 110]byte // FIXME: Split out remaining bytes
}
