/*
 * Pure Go SMART library
 * Copyright 2017 Daniel Swarbrick

 * SCSI generic IO functions
 * TODO:
 * - SCSI CDBs cannot be simplified down to just 6, 10, 12 and 16 byte CDBs
 * - Bytes that make up the CDB have different meanings depending on whether it's a read or write,
 *   or what specific command it is, and thus cannot be simply mapped to a struct.
 */

package smart

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	SG_DXFER_NONE        = -1
	SG_DXFER_TO_DEV      = -2
	SG_DXFER_FROM_DEV    = -3
	SG_DXFER_TO_FROM_DEV = -4

	SG_IO = 0x2285

	// SCSI commands used by this package
	SCSI_INQUIRY         = 0x12
	SCSI_ATA_PASSTHRU_16 = 0x85

	INQ_REPLY_LEN = 36 // Minimum length of standard INQUIRY response
)

// SCSI generic IO
type sgIoHdr struct {
	interface_id    int32
	dxfer_direction int32
	cmd_len         uint8
	mx_sb_len       uint8
	iovec_count     uint16
	dxfer_len       uint32
	dxferp          uintptr
	cmdp            uintptr // Command pointer
	sbp             uintptr // Sense buf pointer
	timeout         uint32
	flags           uint32
	pack_id         int32
	usr_ptr         uintptr
	status          uint8
	masked_status   uint8
	msg_status      uint8
	sb_len_wr       uint8
	host_status     uint16
	driver_status   uint16
	resid           int32
	duration        uint32
	info            uint32
}

// SCSICDB6 is a 6-byte SCSI command descriptor block
type SCSICDB6 struct {
	OpCode    uint8
	Lun       uint8
	Reserved1 uint8
	Reserved2 uint8
	AllocLen  uint8
	Control   uint8
}

type SCSIDevice struct {
	fd int
}

func openDevice(device string) (SCSIDevice, error) {
	var (
		d   SCSIDevice
		err error
	)

	d.fd, err = syscall.Open(device, syscall.O_RDWR, 0600)
	return d, err
}

func (d *SCSIDevice) close() {
	syscall.Close(d.fd)
}

func (d *SCSIDevice) execGenericIO(hdr *sgIoHdr) error {
	if err := ioctl(uintptr(d.fd), SG_IO, uintptr(unsafe.Pointer(hdr))); err != nil {
		return err
	}

	// See http://www.t10.org/lists/2status.htm for SCSI status codes
	if hdr.status != 0 {
		return fmt.Errorf("SCSI generic ioctl returned non-zero status: %#02x", hdr.status)
	}

	return nil
}
