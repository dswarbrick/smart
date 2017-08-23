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

// SATDevice is a simple wrapper around an embedded SCSIDevice type, which handles sending ATA
// commands via SCSI pass-through (SCSI-ATA Translation).
type SATDevice struct {
	SCSIDevice
}

func (d *SATDevice) identify() (*IdentifyDeviceData, error) {
	var identBuf IdentifyDeviceData

	senseBuf := make([]byte, 32)

	cdb16 := CDB16{SCSI_ATA_PASSTHRU_16}
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

	if err := d.execGenericIO(&io_hdr); err != nil {
		fmt.Printf("Sense buffer: % x\n", senseBuf[:io_hdr.sb_len_wr])
		return nil, fmt.Errorf("SgExecute ATA IDENTIFY: %v", err)
	}

	swapBytes(identBuf.SerialNumber[:])
	swapBytes(identBuf.FirmwareRevision[:])
	swapBytes(identBuf.ModelNumber[:])

	return &identBuf, nil
}

// Read SMART log page (WIP / experimental)
func (d *SATDevice) readSMARTLog(logPage uint8) ([]byte, error) {
	senseBuf := make([]byte, 32)
	respBuf := make([]byte, 512)

	cdb16 := CDB16{SCSI_ATA_PASSTHRU_16}
	cdb16[1] = 0x08           // ATA protocol (4 << 1, PIO data-in)
	cdb16[2] = 0x0e           // BYT_BLOK = 1, T_LENGTH = 2, T_DIR = 1
	cdb16[4] = SMART_READ_LOG // feature LSB
	cdb16[6] = 0x01           // sector count
	cdb16[8] = logPage        // SMART log page number
	cdb16[10] = 0x4f          // low lba_mid
	cdb16[12] = 0xc2          // low lba_high
	cdb16[14] = ATA_SMART     // command

	io_hdr := sgIoHdr{interface_id: 'S', dxfer_direction: SG_DXFER_FROM_DEV, timeout: DEFAULT_TIMEOUT}
	io_hdr.cmd_len = uint8(len(cdb16))
	io_hdr.mx_sb_len = uint8(len(senseBuf))
	io_hdr.dxfer_len = uint32(len(respBuf))
	io_hdr.dxferp = uintptr(unsafe.Pointer(&respBuf[0]))
	io_hdr.cmdp = uintptr(unsafe.Pointer(&cdb16))
	io_hdr.sbp = uintptr(unsafe.Pointer(&senseBuf[0]))

	if err := d.execGenericIO(&io_hdr); err != nil {
		fmt.Printf("Sense buffer: % x\n", senseBuf[:io_hdr.sb_len_wr])
		return nil, fmt.Errorf("SgExecute SMART READ LOG: %v", err)
	}

	return respBuf, nil
}

func (d *SATDevice) PrintSMART() error {
	// Standard SCSI INQUIRY command
	inqResp, err := d.inquiry()
	if err != nil {
		return fmt.Errorf("SgExecute INQUIRY: %v", err)
	}

	fmt.Println("SCSI INQUIRY:", inqResp)

	identBuf, err := d.identify()
	if err != nil {
		return err
	}

	fmt.Println("\nATA IDENTIFY data follows:")
	fmt.Printf("Serial Number: %s\n", identBuf.SerialNumber)
	fmt.Println("LU WWN Device Id:", identBuf.getWWN())
	fmt.Printf("Firmware Revision: %s\n", identBuf.FirmwareRevision)
	fmt.Printf("Model Number: %s\n", identBuf.ModelNumber)
	fmt.Printf("Rotation Rate: %d\n", identBuf.RotationRate)
	fmt.Printf("SMART support available: %v\n", identBuf.Word87>>14 == 1)
	fmt.Printf("SMART support enabled: %v\n", identBuf.Word85&0x1 != 0)
	fmt.Println("ATA Major Version:", identBuf.getATAMajorVersion())
	fmt.Println("ATA Minor Version:", identBuf.getATAMinorVersion())
	fmt.Println("Transport:", identBuf.getTransport())

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
	cdb16 := CDB16{SCSI_ATA_PASSTHRU_16}
	cdb16[1] = 0x08            // ATA protocol (4 << 1, PIO data-in)
	cdb16[2] = 0x0e            // BYT_BLOK = 1, T_LENGTH = 2, T_DIR = 1
	cdb16[4] = SMART_READ_DATA // feature LSB
	cdb16[10] = 0x4f           // low lba_mid
	cdb16[12] = 0xc2           // low lba_high
	cdb16[14] = ATA_SMART      // command

	senseBuf := make([]byte, 32)
	respBuf := make([]byte, 512)

	io_hdr := sgIoHdr{interface_id: 'S', dxfer_direction: SG_DXFER_FROM_DEV, timeout: DEFAULT_TIMEOUT}
	io_hdr.cmd_len = uint8(len(cdb16))
	io_hdr.mx_sb_len = uint8(len(senseBuf))
	io_hdr.dxfer_len = uint32(len(respBuf))
	io_hdr.dxferp = uintptr(unsafe.Pointer(&respBuf[0]))
	io_hdr.cmdp = uintptr(unsafe.Pointer(&cdb16))
	io_hdr.sbp = uintptr(unsafe.Pointer(&senseBuf[0]))

	if err = d.execGenericIO(&io_hdr); err != nil {
		fmt.Printf("Sense buffer: % x\n", senseBuf[:io_hdr.sb_len_wr])
		return fmt.Errorf("SgExecute SMART READ DATA: %v", err)
	}

	smart := smartPage{}
	binary.Read(bytes.NewBuffer(respBuf[:362]), nativeEndian, &smart)
	printSMARTPage(smart, thisDrive)

	// Read SMART log directory
	logBuf, err := d.readSMARTLog(0x00)
	if err != nil {
		return err
	}

	smartLogDir := smartLogDirectory{}
	binary.Read(bytes.NewBuffer(logBuf), nativeEndian, &smartLogDir)
	fmt.Printf("\nSMART log directory: %+v\n", smartLogDir)

	// Read SMART error log
	logBuf, err = d.readSMARTLog(0x01)
	if err != nil {
		return err
	}

	sumErrLog := smartSummaryErrorLog{}
	binary.Read(bytes.NewBuffer(logBuf), nativeEndian, &sumErrLog)
	fmt.Printf("\nSummary SMART error log: %+v\n", sumErrLog)

	// Read SMART self-test log
	logBuf, err = d.readSMARTLog(0x06)
	if err != nil {
		return err
	}

	selfTestLog := smartSelfTestLog{}
	binary.Read(bytes.NewBuffer(logBuf), nativeEndian, &selfTestLog)
	fmt.Printf("\nSMART self-test log: %+v\n", selfTestLog)

	return nil
}

// TODO: Make this discover NVMe and MegaRAID devices also.
func ScanDevices() []SCSIDevice {
	var devices []SCSIDevice

	// Find all SCSI disk devices
	files, err := filepath.Glob("/dev/sd*[^0-9]")
	if err != nil {
		return devices
	}

	for _, file := range files {
		devices = append(devices, SCSIDevice{Name: file, fd: -1})
	}

	return devices
}
