/*
 * Pure Go SMART library
 * Copyright 2017 Daniel Swarbrick
 */

package smart

import (
	"fmt"
	"path/filepath"
	"unsafe"
)

// Swap bytes in a byte slice
func swapBytes(s []byte) []byte {
	for i := 0; i < len(s); i += 2 {
		s[i], s[i+1] = s[i+1], s[i]
	}

	return s
}

func ReadSMART(device string) error {
	dev, err := openDevice(device)
	if err != nil {
		return fmt.Errorf("Cannot open device %s: %v", device, err)
	}

	defer dev.close()

	io_hdr := sgIoHdr{interface_id: 'S', dxfer_direction: SG_DXFER_FROM_DEV, timeout: 20000}

	inquiry_cdb := SCSICDB6{OpCode: SCSI_INQUIRY, AllocLen: INQ_REPLY_LEN}
	inqBuff := make([]byte, INQ_REPLY_LEN)
	sense_buf := make([]byte, 32)

	io_hdr.cmd_len = uint8(unsafe.Sizeof(inquiry_cdb))
	io_hdr.mx_sb_len = uint8(len(sense_buf))
	io_hdr.dxfer_len = uint32(len(inqBuff))
	io_hdr.dxferp = uintptr(unsafe.Pointer(&inqBuff[0]))
	io_hdr.cmdp = uintptr(unsafe.Pointer(&inquiry_cdb))
	io_hdr.sbp = uintptr(unsafe.Pointer(&sense_buf[0]))

	if err = dev.execGenericIO(&io_hdr); err != nil {
		return fmt.Errorf("SgExecute INQUIRY: %v", err)
	}

	fmt.Printf("SCSI INQUIRY: %.8s  %.16s  %.4s\n", inqBuff[8:], inqBuff[16:], inqBuff[32:])

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

	fmt.Printf("\nSMART READ DATA response: %#v\n\n", resp_buf)

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
