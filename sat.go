// Copyright 2017 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

// SCSI / ATA Translation functions.

package smart

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"unsafe"
)

const (
	// ATA feature register values for SMART
	SMART_READ_DATA     = 0xd0
	SMART_READ_LOG      = 0xd5
	SMART_RETURN_STATUS = 0xda

	// ATA commands
	ATA_SMART           = 0xb0
	ATA_IDENTIFY_DEVICE = 0xec
)

// ATA IDENTIFY DEVICE struct. ATA8-ACS defines this as a page of 16-bit words. Some fields span
// multiple words (e.g., model number), but must (?) be byteswapped. Some fields use less than a
// single word, and are bitmasked together with other fields. Since many of the fields are now
// retired / obsolete, we only define the fields that are currently used by this package.
type IdentifyDeviceData struct {
	GeneralConfig    uint16 // Word 0, general configuration. If bit 15 is zero, device is ATA.
	_                [9]uint16
	SerialNumber     [20]byte // Word 10..19, device serial number, padded with spaces (20h).
	_                [3]uint16
	FirmwareRevision [8]byte  // Word 23..26, device firmware revision, padded with spaces (20h).
	ModelNumber      [40]byte // Word 27..46, device model number, padded with spaces (20h).
	_                [28]uint16
	SATACap          uint16 // Word 76, SATA capabilities.
	SATACapAddl      uint16 // Word 77, SATA additional capabilities.
	_                [8]uint16
	Word85           uint16 // Word 85, supported commands and feature sets.
	_                uint16
	Word87           uint16 // Word 87, supported commands and feature sets.
	_                [129]uint16
	RotationRate     uint16 // Word 217, nominal media rotation rate.
	_                [4]uint16
	TransportMajor   uint16 // Word 222, transport major version number.
	_                [33]uint16
} // 512 bytes

func (d *IdentifyDeviceData) getTransport() (s string) {
	if (d.TransportMajor == 0) || (d.TransportMajor == 0xffff) {
		s = "device does not report transport"
		return
	}

	switch d.TransportMajor >> 12 {
	case 0x0:
		s = "Parallel ATA"
	case 0x1:
		s = "Serial ATA"

		// TODO: Add decoding of current / max SATA speed (word 76, 77)
		switch log2b(uint(d.TransportMajor & 0x0fff)) {
		case 0:
			s += " ATA8-AST"
		case 1:
			s += " SATA 1.0a"
		case 2:
			s += " SATA II Ext"
		case 3:
			s += " SATA 2.5"
		case 4:
			s += " SATA 2.6"
		case 5:
			s += " SATA 3.0"
		case 6:
			s += " SATA 3.1"
		case 7:
			s += " SATA 3.2"
		default:
			s += fmt.Sprintf(" SATA (%#03x)", d.TransportMajor&0x0fff)
		}
	case 0xe:
		s = fmt.Sprintf("PCIe (%#03x)", d.TransportMajor&0x0fff)
	default:
		s = fmt.Sprintf("Unknown (%#04x)", d.TransportMajor)
	}

	return
}

// ReadSMART reads the SMART attributes of a device (ATA command D0h)
func ReadSMART(devName string) error {
	var (
		identBuf IdentifyDeviceData
		senseBuf [32]byte
	)

	dev := newDevice(devName)
	err := dev.open()
	if err != nil {
		return fmt.Errorf("Cannot open device %s: %v", dev.Name, err)
	}

	defer dev.close()

	inqResp, err := dev.inquiry()
	if err != nil {
		//fmt.Printf("Sense buffer: % x\n", senseBuf[:io_hdr.sb_len_wr])
		return fmt.Errorf("SgExecute INQUIRY: %v", err)
	}

	fmt.Println("SCSI INQUIRY:", inqResp)

	cdb16 := CDB16{}

	cdb16 = CDB16{SCSI_ATA_PASSTHRU_16}
	cdb16[1] = 0x08                 // ATA protocol (4 << 1, PIO data-in)
	cdb16[2] = 0x0e                 // BYT_BLOK = 1, T_LENGTH = 2, T_DIR = 1
	cdb16[14] = ATA_IDENTIFY_DEVICE // command

	io_hdr := sgIoHdr{interface_id: 'S', dxfer_direction: SG_DXFER_FROM_DEV, timeout: DEFAULT_TIMEOUT}
	io_hdr.cmd_len = uint8(len(cdb16))
	io_hdr.mx_sb_len = uint8(len(senseBuf))
	io_hdr.dxfer_len = uint32(unsafe.Sizeof(identBuf))
	io_hdr.dxferp = uintptr(unsafe.Pointer(&identBuf))
	io_hdr.cmdp = uintptr(unsafe.Pointer(&cdb16))
	io_hdr.sbp = uintptr(unsafe.Pointer(&senseBuf[0]))

	if err = dev.execGenericIO(&io_hdr); err != nil {
		fmt.Printf("Sense buffer: % x\n", senseBuf[:io_hdr.sb_len_wr])
		return fmt.Errorf("SgExecute ATA IDENTIFY: %v", err)
	}

	fmt.Println("\nATA IDENTIFY data follows:")
	fmt.Printf("Serial Number: %s\n", swapBytes(identBuf.SerialNumber[:]))
	fmt.Printf("Firmware Revision: %s\n", swapBytes(identBuf.FirmwareRevision[:]))
	fmt.Printf("Model Number: %s\n", swapBytes(identBuf.ModelNumber[:]))
	fmt.Printf("Rotation Rate: %d\n", identBuf.RotationRate)
	fmt.Printf("SMART support available: %v\n", identBuf.Word87>>14 == 1)
	fmt.Printf("SMART support enabled: %v\n", identBuf.Word85&0x1 != 0)
	fmt.Printf("Transport: %s\n", identBuf.getTransport())

	db, err := openDriveDb("drivedb.toml")
	if err != nil {
		return err
	}

	thisDrive := db.lookupDrive(identBuf.ModelNumber[:])
	fmt.Printf("Drive DB contains %d entries. Using model: %s\n", len(db.Drives), thisDrive.Family)

	// FIXME: Check that device supports SMART before trying to read data page

	/*
	 * SMART READ DATA
	 */
	cdb16 = CDB16{SCSI_ATA_PASSTHRU_16}
	cdb16[1] = 0x08            // ATA protocol (4 << 1, PIO data-in)
	cdb16[2] = 0x0e            // BYT_BLOK = 1, T_LENGTH = 2, T_DIR = 1
	cdb16[4] = SMART_READ_DATA // feature LSB
	cdb16[10] = 0x4f           // low lba_mid
	cdb16[12] = 0xc2           // low lba_high
	cdb16[14] = ATA_SMART      // command
	respBuf := [512]byte{}

	io_hdr = sgIoHdr{interface_id: 'S', dxfer_direction: SG_DXFER_FROM_DEV, timeout: DEFAULT_TIMEOUT}
	io_hdr.cmd_len = uint8(len(cdb16))
	io_hdr.mx_sb_len = uint8(len(senseBuf))
	io_hdr.dxfer_len = uint32(len(respBuf))
	io_hdr.dxferp = uintptr(unsafe.Pointer(&respBuf))
	io_hdr.cmdp = uintptr(unsafe.Pointer(&cdb16))
	io_hdr.sbp = uintptr(unsafe.Pointer(&senseBuf[0]))

	if err = dev.execGenericIO(&io_hdr); err != nil {
		fmt.Printf("Sense buffer: % x\n", senseBuf[:io_hdr.sb_len_wr])
		return fmt.Errorf("SgExecute SMART READ DATA: %v", err)
	}

	smart := smartPage{}
	binary.Read(bytes.NewBuffer(respBuf[:362]), nativeEndian, &smart)
	printSMART(smart, thisDrive)

	/*
	 * SMART READ LOG (WIP / experimental)
	 */
	cdb16 = CDB16{SCSI_ATA_PASSTHRU_16}
	cdb16[1] = 0x08           // ATA protocol (4 << 1, PIO data-in)
	cdb16[2] = 0x0e           // BYT_BLOK = 1, T_LENGTH = 2, T_DIR = 1
	cdb16[4] = SMART_READ_LOG // feature LSB
	cdb16[6] = 0x01           // sector count
	cdb16[8] = 0x00           // SMART log directory
	cdb16[10] = 0x4f          // low lba_mid
	cdb16[12] = 0xc2          // low lba_high
	cdb16[14] = ATA_SMART     // command

	io_hdr = sgIoHdr{interface_id: 'S', dxfer_direction: SG_DXFER_FROM_DEV, timeout: DEFAULT_TIMEOUT}
	io_hdr.cmd_len = uint8(len(cdb16))
	io_hdr.mx_sb_len = uint8(len(senseBuf))
	io_hdr.dxfer_len = uint32(len(respBuf))
	io_hdr.dxferp = uintptr(unsafe.Pointer(&respBuf))
	io_hdr.cmdp = uintptr(unsafe.Pointer(&cdb16))
	io_hdr.sbp = uintptr(unsafe.Pointer(&senseBuf[0]))

	if err = dev.execGenericIO(&io_hdr); err != nil {
		fmt.Printf("Sense buffer: % x\n", senseBuf[:io_hdr.sb_len_wr])
		return fmt.Errorf("SgExecute SMART READ LOG: %v", err)
	}

	smartLogDir := smartLogDirectory{}
	binary.Read(bytes.NewBuffer(respBuf[:]), nativeEndian, &smartLogDir)
	fmt.Printf("\nSMART log directory: %+v\n", smartLogDir)

	cdb16 = CDB16{SCSI_ATA_PASSTHRU_16}
	cdb16[1] = 0x08           // ATA protocol (4 << 1, PIO data-in)
	cdb16[2] = 0x0e           // BYT_BLOK = 1, T_LENGTH = 2, T_DIR = 1
	cdb16[4] = SMART_READ_LOG // feature LSB
	cdb16[6] = 0x01           // sector count
	cdb16[8] = 0x01           // summary SMART error log
	cdb16[10] = 0x4f          // low lba_mid
	cdb16[12] = 0xc2          // low lba_high
	cdb16[14] = ATA_SMART     // command

	io_hdr = sgIoHdr{interface_id: 'S', dxfer_direction: SG_DXFER_FROM_DEV, timeout: DEFAULT_TIMEOUT}
	io_hdr.cmd_len = uint8(len(cdb16))
	io_hdr.mx_sb_len = uint8(len(senseBuf))
	io_hdr.dxfer_len = uint32(len(respBuf))
	io_hdr.dxferp = uintptr(unsafe.Pointer(&respBuf))
	io_hdr.cmdp = uintptr(unsafe.Pointer(&cdb16))
	io_hdr.sbp = uintptr(unsafe.Pointer(&senseBuf[0]))

	if err = dev.execGenericIO(&io_hdr); err != nil {
		fmt.Printf("Sense buffer: % x\n", senseBuf[:io_hdr.sb_len_wr])
		return fmt.Errorf("SgExecute SMART READ LOG: %v", err)
	}

	sumErrLog := smartSummaryErrorLog{}
	binary.Read(bytes.NewBuffer(respBuf[:]), nativeEndian, &sumErrLog)
	fmt.Printf("\nSummary SMART error log: %+v\n", sumErrLog)

	cdb16 = CDB16{SCSI_ATA_PASSTHRU_16}
	cdb16[1] = 0x08           // ATA protocol (4 << 1, PIO data-in)
	cdb16[2] = 0x0e           // BYT_BLOK = 1, T_LENGTH = 2, T_DIR = 1
	cdb16[4] = SMART_READ_LOG // feature LSB
	cdb16[6] = 0x01           // sector count
	cdb16[8] = 0x06           // SMART self-test log
	cdb16[10] = 0x4f          // low lba_mid
	cdb16[12] = 0xc2          // low lba_high
	cdb16[14] = ATA_SMART     // command

	io_hdr = sgIoHdr{interface_id: 'S', dxfer_direction: SG_DXFER_FROM_DEV, timeout: DEFAULT_TIMEOUT}
	io_hdr.cmd_len = uint8(len(cdb16))
	io_hdr.mx_sb_len = uint8(len(senseBuf))
	io_hdr.dxfer_len = uint32(len(respBuf))
	io_hdr.dxferp = uintptr(unsafe.Pointer(&respBuf))
	io_hdr.cmdp = uintptr(unsafe.Pointer(&cdb16))
	io_hdr.sbp = uintptr(unsafe.Pointer(&senseBuf[0]))

	if err = dev.execGenericIO(&io_hdr); err != nil {
		fmt.Printf("Sense buffer: % x\n", senseBuf[:io_hdr.sb_len_wr])
		return fmt.Errorf("SgExecute SMART READ LOG: %v", err)
	}

	selfTestLog := smartSelfTestLog{}
	binary.Read(bytes.NewBuffer(respBuf[:]), nativeEndian, &selfTestLog)
	fmt.Printf("\nSMART self-test log: %+v\n", selfTestLog)

	return nil
}

func ScanDevices() []SCSIDevice {
	var devices []SCSIDevice

	// Find all SCSI disk devices
	files, err := filepath.Glob("/dev/sd*[^0-9]")
	if err != nil {
		return devices
	}

	for _, file := range files {
		devices = append(devices, newDevice(file))
	}

	return devices
}

// Swap bytes in a byte slice
func swapBytes(s []byte) []byte {
	for i := 0; i < len(s); i += 2 {
		s[i], s[i+1] = s[i+1], s[i]
	}

	return s
}
