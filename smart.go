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

	fmt.Printf("INQUIRY: %.8s  %.16s  %.4s\n", inqBuff[8:], inqBuff[16:], inqBuff[32:])

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
