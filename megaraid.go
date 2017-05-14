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
 * - Use newer MR_DCMD_PD_LIST_QUERY if possible
 */

package smart

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const (
	SYSFS_SCSI_HOST_DIR = "/sys/class/scsi_host"

	MAX_IOCTL_SGE = 16

	MFI_CMD_PD_SCSI_IO = 0x04
	MFI_CMD_DCMD       = 0x05

	MR_DCMD_CTRL_GET_INFO = 0x01010000
	MR_DCMD_PD_GET_LIST   = 0x02010000 // Obsolete / deprecated command
	MR_DCMD_PD_LIST_QUERY = 0x02010100

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

// megasas_iocpacket struct - caution: megasas driver expects packet struct
type megasas_iocpacket struct {
	host_no   uint16
	__pad1    uint16
	sgl_off   uint32
	sge_count uint32
	sense_off uint32
	sense_len uint32
	frame     [128]byte // FIXME: actually a union of megasas_header / megasas_pthru_frame / megasas_dcmd_frame
	sgl       [MAX_IOCTL_SGE]Iovec
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
	MEGASAS_IOC_FIRMWARE = _iowr('M', 1, uintptr(binary.Size(megasas_iocpacket{})))
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
	binary.Write(b, nativeEndian, ioc)
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
	var respBuf []byte

	m, _ := CreateMegasasIoctl()
	fmt.Printf("%#v\n", m)

	defer m.Close()

	// FIXME: Don't assume that host is always zero
	host := uint16(0)

	// FIXME: Obtain this from user and verify that it's a valid device ID
	diskNum := uint8(26)

	// Send ATA IDENTIFY command as a CDB16 passthru command
	cdb := CDB16{SCSI_ATA_PASSTHRU_16}
	cdb[1] = 0x08                 // ATA protocol (4 << 1, PIO data-in)
	cdb[2] = 0x0e                 // BYT_BLOK = 1, T_LENGTH = 2, T_DIR = 1
	cdb[14] = ATA_IDENTIFY_DEVICE // command
	respBuf = make([]byte, 512)
	m.PassThru(host, diskNum, cdb[:], respBuf, SG_DXFER_FROM_DEV)

	ident_buf := IdentifyDeviceData{}
	binary.Read(bytes.NewBuffer(respBuf), nativeEndian, &ident_buf)

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

	// Send ATA SMART READ command as a CDB16 passthru command
	cdb = CDB16{SCSI_ATA_PASSTHRU_16}
	cdb[1] = 0x08            // ATA protocol (4 << 1, PIO data-in)
	cdb[2] = 0x0e            // BYT_BLOK = 1, T_LENGTH = 2, T_DIR = 1
	cdb[4] = SMART_READ_DATA // feature LSB
	cdb[10] = 0x4f           // low lba_mid
	cdb[12] = 0xc2           // low lba_high
	cdb[14] = ATA_SMART      // command
	respBuf = make([]byte, 512)
	m.PassThru(host, diskNum, cdb[:], respBuf, SG_DXFER_FROM_DEV)

	smart := smartPage{}
	binary.Read(bytes.NewBuffer(respBuf[:362]), nativeEndian, &smart)
	printSMART(smart, thisDrive)

	return nil
}

// Scan system for MegaRAID adapters and their devices
func MegaScan() {
	m, _ := CreateMegasasIoctl()
	fmt.Printf("%#v\n", m)

	defer m.Close()

	files, err := ioutil.ReadDir(SYSFS_SCSI_HOST_DIR)
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range files {
		if file.Mode()&os.ModeSymlink != 0 {
			b, err := ioutil.ReadFile(filepath.Join(SYSFS_SCSI_HOST_DIR, file.Name(), "proc_name"))
			if err != nil {
				continue
			}

			if string(bytes.Trim(b, "\n")) == "megaraid_sas" {
				var hostNum uint16

				if _, err := fmt.Sscanf(file.Name(), "host%d", &hostNum); err != nil {
					continue
				}

				devices, _ := m.GetDeviceList(hostNum)

				fmt.Println("\nEncl.  Slot  Device Id  SAS Address")
				for _, pd := range devices {
					if pd.SCSIDevType == 0 { // SCSI disk
						fmt.Printf("%5d   %3d      %5d  %#x\n", pd.EnclosureId, pd.SlotNumber, pd.DeviceId, pd.SASAddr[0])
					}
				}

				fmt.Println()

				for _, pd := range devices {
					if pd.SCSIDevType == 0 { // SCSI disk
						var inqBuf inquiryResponse

						cdb := CDB6{SCSI_INQUIRY}
						binary.BigEndian.PutUint16(cdb[3:], INQ_REPLY_LEN)

						respBuf := make([]byte, 512)
						m.PassThru(0, uint8(pd.DeviceId), cdb[:], respBuf, SG_DXFER_FROM_DEV)

						binary.Read(bytes.NewReader(respBuf), nativeEndian, &inqBuf)
						fmt.Printf("diskNum: %d  INQUIRY data: %s\n", pd.DeviceId, inqBuf)
					}
				}
			}
		}
	}
}
