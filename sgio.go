/*
 * Pure Go SMART library
 * Copyright 2017 Daniel Swarbrick

 * SCSI generic IO functions
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

	SG_INFO_OK_MASK = 0x1
	SG_INFO_OK      = 0x0

	SG_IO = 0x2285

	// SCSI commands used by this package
	SCSI_INQUIRY         = 0x12
	SCSI_ATA_PASSTHRU_16 = 0x85

	INQ_REPLY_LEN = 36 // Minimum length of standard INQUIRY response
)

// SCSI generic IO, analogous to sg_io_hdr_t
type sgIoHdr struct {
	interface_id    int32   // 'S' for SCSI generic (required)
	dxfer_direction int32   // data transfer direction
	cmd_len         uint8   // SCSI command length (<= 16 bytes)
	mx_sb_len       uint8   // max length to write to sbp
	iovec_count     uint16  // 0 implies no scatter gather
	dxfer_len       uint32  // byte count of data transfer
	dxferp          uintptr // points to data transfer memory or scatter gather list
	cmdp            uintptr // points to command to perform
	sbp             uintptr // points to sense_buffer memory
	timeout         uint32  // MAX_UINT -> no timeout (unit: millisec)
	flags           uint32  // 0 -> default, see SG_FLAG...
	pack_id         int32   // unused internally (normally)
	usr_ptr         uintptr // unused internally
	status          uint8   // SCSI status
	masked_status   uint8   // shifted, masked scsi status
	msg_status      uint8   // messaging level data (optional)
	sb_len_wr       uint8   // byte count actually written to sbp
	host_status     uint16  // errors from host adapter
	driver_status   uint16  // errors from software driver
	resid           int32   // dxfer_len - actual_transferred
	duration        uint32  // time taken by cmd (unit: millisec)
	info            uint32  // auxiliary information
}

// SCSI CDB types
type CDB6 [6]byte
type CDB16 [16]byte

// SCSI INQUIRY response
type inquiryResponse struct {
	Peripheral   byte // peripheral qualifier, device type
	_            byte
	Version      byte
	_            [5]byte
	VendorIdent  [8]byte
	ProductIdent [16]byte
	ProductRev   [4]byte
}

func (inq inquiryResponse) String() string {
	return fmt.Sprintf("%.8s  %.16s  %.4s", inq.VendorIdent, inq.ProductIdent, inq.ProductRev)
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
	if hdr.info&SG_INFO_OK_MASK != SG_INFO_OK {
		return fmt.Errorf("SCSI status: %#02x, host status: %#02x, driver status: %#02x",
			hdr.status, hdr.host_status, hdr.driver_status)
	}

	return nil
}
