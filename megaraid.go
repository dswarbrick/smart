/*
 * Pure Go SMART library
 * Copyright 2017 Daniel Swarbrick
 *
 * Broadcom (formerly Avago, LSI) MegaRAID ioctl functions
 * TODO:
 * - Improve code comments, refer to in-kernel structs
 * - Device Scan:
 *   - Walk /sys/class/scsi_host/ directory
 *   - "host%d" symlinks enumerate hosts
 *   - "host%d/proc_name" should contain the value "megaraid_sas"
 */

package smart

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

const (
	MAX_IOCTL_SGE = 16

	MFI_CMD_PD_SCSI_IO = 0x04
	MFI_CMD_DCMD       = 0x05

	MR_DCMD_PD_GET_LIST = 0x02010000 // Obsolete / deprecated command

	MFI_FRAME_DIR_NONE  = 0x0000
	MFI_FRAME_DIR_WRITE = 0x0008
	MFI_FRAME_DIR_READ  = 0x0010
	MFI_FRAME_DIR_BOTH  = 0x0018
)

type megasas_sge64 struct {
	phys_addr uint32
	length    uint32
	_padding  uint32
}

type Iovec struct {
	Base uint64 // FIXME: This is not portable to 32-bit platforms!
	Len  uint64
}

type megasas_dcmd_frame struct {
	cmd           uint8
	reserved_0    uint8
	cmd_status    uint8
	reserved_1    [4]uint8
	sge_count     uint8
	context       uint32
	pad_0         uint32
	flags         uint16
	timeout       uint16
	data_xfer_len uint32
	opcode        uint32
	mbox          [12]byte      // FIXME: This is actually a union of [12]uint8 / [6]uint16 / [3]uint32
	sgl           megasas_sge64 // FIXME: This is actually a union of megasas_sge64 / megasas_sge32
}

type megasas_pthru_frame struct {
	cmd                    uint8
	sense_len              uint8
	cmd_status             uint8
	scsi_status            uint8
	target_id              uint8
	lun                    uint8
	cdb_len                uint8
	sge_count              uint8
	context                uint32
	pad_0                  uint32
	flags                  uint16
	timeout                uint16
	data_xfer_len          uint32
	sense_buf_phys_addr_lo uint32
	sense_buf_phys_addr_hi uint32
	cdb                    [16]byte
	sgl                    megasas_sge64
}

type megasas_iocpacket struct {
	host_no   uint16
	__pad1    uint16
	sgl_off   uint32
	sge_count uint32
	sense_off uint32
	sense_len uint32
	// FIXME: This is actually a union of megasas_header / megasas_pthru_frame / megasas_dcmd_frame
	frame [128]byte
	// FIXME: Go is inserting 4 bytes of padding before this in order to 64-bit align the sgl member
	sgl [MAX_IOCTL_SGE]Iovec
}

// Megasas physical device address
type MegasasPDAddress struct {
	DeviceId          uint16
	EnclosureId       uint16
	EnclosureIndex    uint8
	SlotNumber        uint8
	SCSIDevType       uint8
	ConnectPortBitmap uint8
	SASAddr           [2]uint64
}

// Holder for megasas ioctl device
type MegasasIoctl struct {
	DeviceMajor int
	fd          int
}

var (
	// 0xc1944d01 - Beware: cannot use unsafe.Sizeof(megasas_iocpacket{}) due to Go struct padding!
	MEGASAS_IOC_FIRMWARE = _iowr('M', 1, 404)
)

// MakeDev returns the device ID for the specified major and minor numbers, equivalent to
// makedev(3). Based on gnu_dev_makedev macro, may be platform dependent!
func MakeDev(major, minor uint) uint {
	return (minor & 0xff) | ((major & 0xfff) << 8) |
		((minor &^ 0xff) << 12) | ((major &^ 0xfff) << 32)
}

// PackedBytes is a convenience method that will pack a megasas_iocpacket struct in little-endian
// format and return it as a byte slice
func (ioc *megasas_iocpacket) PackedBytes() []byte {
	b := new(bytes.Buffer)
	binary.Write(b, binary.LittleEndian, ioc)
	return b.Bytes()
}

// CreateMegasasIoctl determines the device ID for the MegaRAID SAS ioctl device, creates it
// if necessary, and returns a MegasasIoctl struct to manage the device.
func CreateMegasasIoctl() (MegasasIoctl, error) {
	var (
		m   MegasasIoctl
		err error
	)

	// megaraid_sas driver does not automatically create ioctl device node, so find out the device
	// major number and create it.
	if file, err := os.Open("/proc/devices"); err == nil {
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			if strings.HasSuffix(scanner.Text(), "megaraid_sas_ioctl") {
				if _, err := fmt.Sscanf(scanner.Text(), "%d", &m.DeviceMajor); err == nil {
					break
				}
			}
		}

		if m.DeviceMajor == 0 {
			log.Println("Could not determine megaraid major number!")
			return m, nil
		}

		syscall.Mknod("/dev/megaraid_sas_ioctl_node", syscall.S_IFCHR, int(MakeDev(uint(m.DeviceMajor), 0)))
	} else {
		return m, err
	}

	m.fd, err = syscall.Open("/dev/megaraid_sas_ioctl_node", syscall.O_RDWR, 0600)

	if err != nil {
		return m, err
	}

	return m, nil
}

// Close closes the file descriptor of the MegasasIoctl instance
func (m *MegasasIoctl) Close() {
	syscall.Close(m.fd)
}

// MFI sends a MegaRAID Firmware Interface (MFI) command to the specified host
func (m *MegasasIoctl) MFI(host uint16, opcode uint32, b []byte) error {
	ioc := megasas_iocpacket{host_no: host}

	// Approximation of C union behaviour
	dcmd := (*megasas_dcmd_frame)(unsafe.Pointer(&ioc.frame))
	dcmd.cmd = MFI_CMD_DCMD
	dcmd.opcode = opcode
	dcmd.data_xfer_len = uint32(len(b))
	dcmd.sge_count = 1

	ioc.sge_count = 1
	ioc.sgl_off = uint32(unsafe.Offsetof(dcmd.sgl))
	ioc.sgl[0] = Iovec{uint64(uintptr(unsafe.Pointer(&b[0]))), uint64(len(b))}

	iocBuf := ioc.PackedBytes()

	// Note pointer to first item in iocBuf buffer
	if err := ioctl(uintptr(m.fd), MEGASAS_IOC_FIRMWARE, uintptr(unsafe.Pointer(&iocBuf[0]))); err != nil {
		return err
	}

	return nil
}

// PassThru sends a SCSI command to a MegaRAID controller
func (m *MegasasIoctl) PassThru(host uint16, diskNum uint8, cdb []byte, buf []byte, dxfer_dir int) {
	ioc := megasas_iocpacket{host_no: host}

	// Approximation of C union behaviour
	pthru := (*megasas_pthru_frame)(unsafe.Pointer(&ioc.frame))
	pthru.cmd_status = 0xff
	pthru.cmd = MFI_CMD_PD_SCSI_IO
	pthru.target_id = diskNum
	pthru.cdb_len = uint8(len(cdb))

	// FIXME: Don't use SG_* here
	switch dxfer_dir {
	case SG_DXFER_NONE:
		pthru.flags = MFI_FRAME_DIR_NONE
	case SG_DXFER_FROM_DEV:
		pthru.flags = MFI_FRAME_DIR_READ
	case SG_DXFER_TO_DEV:
		pthru.flags = MFI_FRAME_DIR_WRITE
	}

	copy(pthru.cdb[:], cdb)

	pthru.data_xfer_len = uint32(len(buf))
	pthru.sge_count = 1

	ioc.sge_count = 1
	ioc.sgl_off = uint32(unsafe.Offsetof(pthru.sgl))
	ioc.sgl[0] = Iovec{uint64(uintptr(unsafe.Pointer(&buf[0]))), uint64(len(buf))}

	iocBuf := ioc.PackedBytes()

	// Note pointer to first item in iocBuf buffer
	if err := ioctl(uintptr(m.fd), MEGASAS_IOC_FIRMWARE, uintptr(unsafe.Pointer(&iocBuf[0]))); err != nil {
		log.Fatal(err)
	}
}

// GetDeviceList retrieves a list of physical devices attached to the specified host
func (m *MegasasIoctl) GetDeviceList(host uint16) ([]MegasasPDAddress, error) {
	respBuf := make([]byte, 4096)

	if err := m.MFI(0, MR_DCMD_PD_GET_LIST, respBuf); err != nil {
		log.Println(err)
		return nil, err
	}

	respCount := nativeEndian.Uint32(respBuf[4:])

	// Create a device array large enough to hold the specified number of devices
	devices := make([]MegasasPDAddress, respCount)
	binary.Read(bytes.NewBuffer(respBuf[8:]), nativeEndian, &devices)

	return devices, nil
}

func OpenMegasasIoctl() error {
	var cdb []byte

	m, _ := CreateMegasasIoctl()
	fmt.Printf("%#v\n", m)

	defer m.Close()

	// FIXME: Don't assume that host is always zero
	devices, _ := m.GetDeviceList(0)

	fmt.Println("\nEncl.  Slot  Device Id  SAS Address")
	for _, pd := range devices {
		if pd.SCSIDevType == 0 { // SCSI disk
			fmt.Printf("%5d   %3d      %5d  %#x\n", pd.EnclosureId, pd.SlotNumber, pd.DeviceId, pd.SASAddr[0])
		}
	}

	fmt.Println()

	for _, pd := range devices {
		if pd.SCSIDevType == 0 { // SCSI disk
			cdb = []byte{SCSI_INQUIRY, 0, 0, 0, INQ_REPLY_LEN, 0}
			resp := make([]byte, 512)
			m.PassThru(0, uint8(pd.DeviceId), cdb, resp, SG_DXFER_FROM_DEV)
			fmt.Printf("diskNum: %d  INQUIRY data: %.8s  %.16s  %.4s\n", pd.DeviceId, resp[8:], resp[16:], resp[32:])
		}
	}

	fmt.Println()

	// Send ATA IDENTIFY command as a CDB16 passthru command
	cdb = []byte{SCSI_ATA_PASSTHRU_16, 0x08, 0x0e, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xec, 0x00}
	buf := make([]byte, 512)
	m.PassThru(0, 26, cdb, buf, SG_DXFER_FROM_DEV)

	ident_buf := IdentifyDeviceData{}
	binary.Read(bytes.NewBuffer(buf), binary.LittleEndian, &ident_buf)

	fmt.Printf("Serial Number: %s\n", swapBytes(ident_buf.SerialNumber[:]))
	fmt.Printf("Firmware Revision: %s\n", swapBytes(ident_buf.FirmwareRevision[:]))
	fmt.Printf("Model Number: %s\n", swapBytes(ident_buf.ModelNumber[:]))

	return nil
}
