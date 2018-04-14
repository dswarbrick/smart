// Copyright 2017-18 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

// Broadcom (formerly Avago, LSI) MegaRAID ioctl functions.
// TODO:
//  - Improve code comments, refer to in-kernel structs
//  - Use newer MR_DCMD_PD_LIST_QUERY if possible

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
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/dswarbrick/smart/ata"
	"github.com/dswarbrick/smart/drivedb"
	"github.com/dswarbrick/smart/ioctl"
	"github.com/dswarbrick/smart/scsi"
	"github.com/dswarbrick/smart/utils"
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

// Holder for megaraid_sas ioctl device
type MegasasIoctl struct {
	DeviceMajor uint32
	fd          int
}

type MegasasDevice struct {
	Name     string
	hostNum  uint16
	deviceId uint16
	ctl      *MegasasIoctl
}

var (
	// 0xc1944d01 - Beware: cannot use unsafe.Sizeof(megasas_iocpacket{}) due to Go struct padding!
	MEGASAS_IOC_FIRMWARE = ioctl.Iowr('M', 1, uintptr(binary.Size(megasas_iocpacket{})))
)

// PackedBytes is a convenience method that will pack a megasas_iocpacket struct in little-endian
// format and return it as a byte slice
func (ioc *megasas_iocpacket) PackedBytes() []byte {
	b := new(bytes.Buffer)
	binary.Write(b, utils.NativeEndian, ioc)
	return b.Bytes()
}

// CreateMegasasIoctl determines the device ID for the MegaRAID SAS ioctl device, creates it
// if necessary, and returns a MegasasIoctl struct to interact with the megaraid_sas driver.
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

		unix.Mknod("/dev/megaraid_sas_ioctl_node", unix.S_IFCHR, int(unix.Mkdev(m.DeviceMajor, 0)))
	} else {
		return m, err
	}

	m.fd, err = unix.Open("/dev/megaraid_sas_ioctl_node", unix.O_RDWR, 0600)

	if err != nil {
		return m, err
	}

	return m, nil
}

// Close closes the file descriptor of the MegasasIoctl instance
func (m *MegasasIoctl) Close() {
	unix.Close(m.fd)
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
	if err := ioctl.Ioctl(uintptr(m.fd), MEGASAS_IOC_FIRMWARE, uintptr(unsafe.Pointer(&iocBuf[0]))); err != nil {
		return err
	}

	return nil
}

// PassThru sends a SCSI command to a MegaRAID controller
func (m *MegasasIoctl) PassThru(host uint16, diskNum uint8, cdb []byte, buf []byte, dxfer_dir int) error {
	ioc := megasas_iocpacket{host_no: host}

	// Approximation of C union behaviour
	pthru := (*megasas_pthru_frame)(unsafe.Pointer(&ioc.frame))
	pthru.cmd_status = 0xff
	pthru.cmd = MFI_CMD_PD_SCSI_IO
	pthru.target_id = diskNum
	pthru.cdb_len = uint8(len(cdb))

	// FIXME: Don't use SG_* here
	switch dxfer_dir {
	case scsi.SG_DXFER_NONE:
		pthru.flags = MFI_FRAME_DIR_NONE
	case scsi.SG_DXFER_FROM_DEV:
		pthru.flags = MFI_FRAME_DIR_READ
	case scsi.SG_DXFER_TO_DEV:
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
	return ioctl.Ioctl(uintptr(m.fd), MEGASAS_IOC_FIRMWARE, uintptr(unsafe.Pointer(&iocBuf[0])))
}

// GetPDList retrieves a list of physical devices attached to the specified host
func (m *MegasasIoctl) GetPDList(host uint16) ([]MegasasPDAddress, error) {
	respBuf := make([]byte, 4096)

	if err := m.MFI(0, MR_DCMD_PD_GET_LIST, respBuf); err != nil {
		log.Println(err)
		return nil, err
	}

	respCount := utils.NativeEndian.Uint32(respBuf[4:])

	// Create a device array large enough to hold the specified number of devices
	devices := make([]MegasasPDAddress, respCount)
	binary.Read(bytes.NewBuffer(respBuf[8:]), utils.NativeEndian, &devices)

	return devices, nil
}

// ScanHosts scans system for megaraid_sas controllers and returns a slice of host numbers
func (m *MegasasIoctl) ScanHosts() ([]uint16, error) {
	var hosts []uint16

	files, err := ioutil.ReadDir(SYSFS_SCSI_HOST_DIR)
	if err != nil {
		return hosts, err
	}

	for _, file := range files {
		if file.Mode()&os.ModeSymlink != 0 {
			b, err := ioutil.ReadFile(filepath.Join(SYSFS_SCSI_HOST_DIR, file.Name(), "proc_name"))
			if err != nil {
				continue
			}

			if string(bytes.Trim(b, "\n")) == "megaraid_sas" {
				var hostNum uint16

				if _, err := fmt.Sscanf(file.Name(), "host%d", &hostNum); err == nil {
					hosts = append(hosts, hostNum)
				}
			}
		}
	}

	return hosts, nil
}

// ScanDevices scans systme for (presumably) SMART-capable devices on all available host adapters
func (m *MegasasIoctl) ScanDevices() []MegasasDevice {
	var mdevs []MegasasDevice

	hosts, _ := m.ScanHosts()
	for _, hostNum := range hosts {
		devices, _ := m.GetPDList(hostNum)
		for _, pd := range devices {
			if pd.SCSIDevType == 0 { // SCSI disk
				md := MegasasDevice{
					Name:     fmt.Sprintf("megaraid%d_%d", hostNum, pd.DeviceId),
					hostNum:  hostNum,
					deviceId: pd.DeviceId,
					ctl:      m,
				}
				mdevs = append(mdevs, md)
			}
		}
	}

	return mdevs
}

// inquiry fetches a standard SCSI INQUIRY response from device
// TODO: Return error if unsuccessful
func (d *MegasasDevice) inquiry() scsi.InquiryResponse {
	var inqBuf scsi.InquiryResponse

	cdb := scsi.CDB6{scsi.SCSI_INQUIRY}
	binary.BigEndian.PutUint16(cdb[3:], scsi.INQ_REPLY_LEN)

	respBuf := make([]byte, 512)
	if err := d.ctl.PassThru(d.hostNum, uint8(d.deviceId), cdb[:], respBuf, scsi.SG_DXFER_FROM_DEV); err != nil {
		return inqBuf
	}

	binary.Read(bytes.NewReader(respBuf), utils.NativeEndian, &inqBuf)
	return inqBuf
}

func OpenMegasasIoctl(host uint16, diskNum uint8) error {
	var respBuf []byte

	m, _ := CreateMegasasIoctl()
	fmt.Printf("%#v\n", m)

	defer m.Close()

	md := MegasasDevice{
		Name:     fmt.Sprintf("megaraid%d_%d", host, diskNum),
		hostNum:  host,
		deviceId: uint16(diskNum),
		ctl:      &m,
	}
	fmt.Printf("%#v\n", md)

	// Send ATA IDENTIFY command as a CDB16 passthru command
	cdb := scsi.CDB16{scsi.SCSI_ATA_PASSTHRU_16}
	cdb[1] = 0x08                 // ATA protocol (4 << 1, PIO data-in)
	cdb[2] = 0x0e                 // BYT_BLOK = 1, T_LENGTH = 2, T_DIR = 1
	cdb[14] = ATA_IDENTIFY_DEVICE // command
	respBuf = make([]byte, 512)

	if err := m.PassThru(host, diskNum, cdb[:], respBuf, scsi.SG_DXFER_FROM_DEV); err != nil {
		return err
	}

	ident_buf := ata.IdentifyDeviceData{}
	binary.Read(bytes.NewBuffer(respBuf), utils.NativeEndian, &ident_buf)

	fmt.Println("\nATA IDENTIFY data follows:")
	fmt.Printf("Serial Number: %s\n", ident_buf.SerialNumber())
	fmt.Printf("Firmware Revision: %s\n", ident_buf.FirmwareRevision())
	fmt.Printf("Model Number: %s\n", ident_buf.ModelNumber())

	db, err := drivedb.OpenDriveDb("drivedb.toml")
	if err != nil {
		return err
	}

	thisDrive := db.LookupDrive(ident_buf.ModelNumber())
	fmt.Printf("Drive DB contains %d entries. Using model: %s\n", len(db.Drives), thisDrive.Family)

	// Send ATA SMART READ command as a CDB16 passthru command
	cdb = scsi.CDB16{scsi.SCSI_ATA_PASSTHRU_16}
	cdb[1] = 0x08            // ATA protocol (4 << 1, PIO data-in)
	cdb[2] = 0x0e            // BYT_BLOK = 1, T_LENGTH = 2, T_DIR = 1
	cdb[4] = SMART_READ_DATA // feature LSB
	cdb[10] = 0x4f           // low lba_mid
	cdb[12] = 0xc2           // low lba_high
	cdb[14] = ATA_SMART      // command
	respBuf = make([]byte, 512)

	if err := m.PassThru(host, diskNum, cdb[:], respBuf, scsi.SG_DXFER_FROM_DEV); err != nil {
		return err
	}

	smart := smartPage{}
	binary.Read(bytes.NewBuffer(respBuf[:362]), utils.NativeEndian, &smart)
	printSMARTPage(smart, thisDrive)

	return nil
}

// Scan system for MegaRAID adapters and their devices
func MegaScan() {
	m, _ := CreateMegasasIoctl()
	defer m.Close()

	hosts, _ := m.ScanHosts()
	for _, hostNum := range hosts {
		devices, _ := m.GetPDList(hostNum)

		fmt.Println("\nEncl.  Slot  Device Id  SAS Address")
		for _, pd := range devices {
			if pd.SCSIDevType == 0 { // SCSI disk
				fmt.Printf("%5d   %3d      %5d  %#x\n", pd.EnclosureId, pd.SlotNumber, pd.DeviceId, pd.SASAddr[0])
			}
		}

		fmt.Println()

		for _, pd := range devices {
			if pd.SCSIDevType == 0 { // SCSI disk
				md := MegasasDevice{
					Name:     fmt.Sprintf("megaraid%d_%d", hostNum, pd.DeviceId),
					hostNum:  hostNum,
					deviceId: uint16(pd.DeviceId),
					ctl:      &m,
				}

				fmt.Printf("diskNum: %d  INQUIRY data: %s\n", pd.DeviceId, md.inquiry())
			}
		}
	}
}
