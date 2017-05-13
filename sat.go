/*
 * Pure Go SMART library
 * Copyright 2017 Daniel Swarbrick
 *
 * SCSI / ATA Translation functions
 */

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
	SMART_READ_DATA = 0xd0

	// ATA commands
	ATA_SMART           = 0xb0
	ATA_IDENTIFY_DEVICE = 0xec
)

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

func ReadSMART(device string) error {
	var inqBuff inquiryResponse

	dev, err := openDevice(device)
	if err != nil {
		return fmt.Errorf("Cannot open device %s: %v", device, err)
	}

	defer dev.close()

	io_hdr := sgIoHdr{interface_id: 'S', dxfer_direction: SG_DXFER_FROM_DEV, timeout: 20000}

	inquiry_cdb := SCSICDB6{OpCode: SCSI_INQUIRY, AllocLen: uint8(unsafe.Sizeof(inqBuff))}
	sense_buf := make([]byte, 32)

	io_hdr.cmd_len = uint8(unsafe.Sizeof(inquiry_cdb))
	io_hdr.mx_sb_len = uint8(len(sense_buf))
	io_hdr.dxfer_len = uint32(unsafe.Sizeof(inqBuff))
	io_hdr.dxferp = uintptr(unsafe.Pointer(&inqBuff))
	io_hdr.cmdp = uintptr(unsafe.Pointer(&inquiry_cdb))
	io_hdr.sbp = uintptr(unsafe.Pointer(&sense_buf[0]))

	if err = dev.execGenericIO(&io_hdr); err != nil {
		return fmt.Errorf("SgExecute INQUIRY: %v", err)
	}

	fmt.Println("SCSI INQUIRY:", inqBuff)

	cdb16 := [16]byte{}

	io_hdr = sgIoHdr{interface_id: 'S', dxfer_direction: SG_DXFER_FROM_DEV, timeout: 20000}
	// 0x08 : ATA protocol (4 << 1, PIO data-in)
	// 0x0e : BYT_BLOK = 1, T_LENGTH = 2, T_DIR = 1
	cdb16 = [16]byte{SCSI_ATA_PASSTHRU_16, 0x08, 0x0e, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, ATA_IDENTIFY_DEVICE, 0x00}
	ident_buf := IdentifyDeviceData{}

	io_hdr.cmd_len = uint8(len(cdb16))
	io_hdr.mx_sb_len = uint8(len(sense_buf))
	io_hdr.dxfer_len = uint32(unsafe.Sizeof(ident_buf))
	io_hdr.dxferp = uintptr(unsafe.Pointer(&ident_buf))
	io_hdr.cmdp = uintptr(unsafe.Pointer(&cdb16))
	io_hdr.sbp = uintptr(unsafe.Pointer(&sense_buf[0]))

	if err = dev.execGenericIO(&io_hdr); err != nil {
		return fmt.Errorf("SgExecute ATA IDENTIFY: %v", err)
	}

	fmt.Println("\nATA IDENTIFY data follows:")
	fmt.Printf("Serial Number: %s\n", swapBytes(ident_buf.SerialNumber[:]))
	fmt.Printf("Firmware Revision: %s\n", swapBytes(ident_buf.FirmwareRevision[:]))
	fmt.Printf("Model Number: %s\n", swapBytes(ident_buf.ModelNumber[:]))

	db, err := openDriveDb("drivedb.toml")
	if err != nil {
		return err
	}

	thisDrive := db.lookupDrive(ident_buf.ModelNumber[:])
	fmt.Printf("Drive DB contains %d entries. Using model: %s\n", len(db.Drives), thisDrive.Family)

	// FIXME: Check that device supports SMART before trying to read data page

	/*
	 * SMART READ DATA
	 * command code B0h, feature register D0h
	 * LBA mid register 4Fh, LBA high register C2h
	 */
	io_hdr = sgIoHdr{interface_id: 'S', dxfer_direction: SG_DXFER_FROM_DEV, timeout: 20000}
	// 0x08 : ATA protocol (4 << 1, PIO data-in)
	// 0x0e : BYT_BLOK = 1, T_LENGTH = 2, T_DIR = 1
	cdb16 = [16]byte{SCSI_ATA_PASSTHRU_16, 0x08, 0x0e, 0x00, SMART_READ_DATA, 0x00, 0x01, 0x00, 0x00, 0x00, 0x4f, 0x00, 0xc2, 0x00, ATA_SMART, 0x00}
	resp_buf := [512]byte{}

	io_hdr.cmd_len = uint8(len(cdb16))
	io_hdr.mx_sb_len = uint8(len(sense_buf))
	io_hdr.dxfer_len = uint32(len(resp_buf))
	io_hdr.dxferp = uintptr(unsafe.Pointer(&resp_buf))
	io_hdr.cmdp = uintptr(unsafe.Pointer(&cdb16))
	io_hdr.sbp = uintptr(unsafe.Pointer(&sense_buf[0]))

	if err = dev.execGenericIO(&io_hdr); err != nil {
		return fmt.Errorf("SgExecute SMART READ DATA: %v", err)
	}

	smart := smartPage{}
	binary.Read(bytes.NewBuffer(resp_buf[:362]), nativeEndian, &smart)
	printSMART(smart, thisDrive)

	return nil
}

func ScanDevices() {
	// Find all SCSI disk devices
	files, err := filepath.Glob("/dev/sd*[^1-9]")
	if err != nil {
		return
	}

	for _, file := range files {
		fmt.Println(file)
	}
}

// Swap bytes in a byte slice
func swapBytes(s []byte) []byte {
	for i := 0; i < len(s); i += 2 {
		s[i], s[i+1] = s[i+1], s[i]
	}

	return s
}
