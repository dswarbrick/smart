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
	fmt.Println("LU WWN Device Id:", identBuf.getWWN())
	fmt.Printf("Firmware Revision: %s\n", swapBytes(identBuf.FirmwareRevision[:]))
	fmt.Printf("Model Number: %s\n", swapBytes(identBuf.ModelNumber[:]))
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
