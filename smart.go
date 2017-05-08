/*
 * Pure Go SMART library
 * Copyright 2017 Daniel Swarbrick
 */

package smart

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unsafe"

	"github.com/BurntSushi/toml"
)

type attrConv struct {
	Conv string
	Name string
}

type driveModel struct {
	Family         string
	ModelRegex     string
	FirmwareRegex  string
	WarningMsg     string
	Presets        map[string]attrConv
	CompiledRegexp *regexp.Regexp
}

type driveDb struct {
	Drives []driveModel
}

var nativeEndian binary.ByteOrder

// Determine native endianness of system
func init() {
	i := uint32(1)
	b := (*[4]byte)(unsafe.Pointer(&i))
	if b[0] == 1 {
		nativeEndian = binary.LittleEndian
	} else {
		nativeEndian = binary.BigEndian
	}
}

// lookupDrive returns the most appropriate driveModel for a given ATA IDENTIFY value
func (db *driveDb) lookupDrive(ident []byte) driveModel {
	var model, defaultModel driveModel

	for _, d := range db.Drives {
		// Skip placeholder entry
		if strings.HasPrefix(d.Family, "$Id") {
			continue
		}

		if d.Family == "DEFAULT" {
			defaultModel = d
			continue
		}

		if d.CompiledRegexp.Match(ident) {
			model = d

			// Inherit presets from defaultModel
			for id, p := range defaultModel.Presets {
				if _, exists := model.Presets[id]; exists {
					// Some drives override the conv but don't specify a name, so copy it from default
					if model.Presets[id].Name == "" {
						model.Presets[id] = attrConv{Name: p.Name, Conv: model.Presets[id].Conv}
					}
				} else {
					model.Presets[id] = p
				}
			}

			break
		}
	}

	return model
}

// openDriveDb opens a .toml formatted drive database, unmarshalls it, and returns a driveDb
func openDriveDb(dbfile string) (driveDb, error) {
	var db driveDb

	if _, err := toml.DecodeFile(dbfile, &db); err != nil {
		return db, fmt.Errorf("Cannot open / parse drive DB: %s", err)
	}

	for i, d := range db.Drives {
		db.Drives[i].CompiledRegexp, _ = regexp.Compile(d.ModelRegex)
	}

	return db, nil
}

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

	fmt.Printf("\nSMART structure version: %d\n", smart.Version)
	fmt.Printf("ID# ATTRIBUTE_NAME           FLAG     VALUE WORST RESERVED RAW_VALUE     VENDOR_BYTES\n")

	for _, attr := range smart.Attrs {
		var rawValue uint64

		if attr.Id == 0 {
			break
		}

		attrconv := thisDrive.Presets[strconv.Itoa(int(attr.Id))]

		switch attrconv.Conv {
		case "raw24(raw8)":
			// Big-endian 24-bit number, optionally with 8 bit values
			for i := 2; i >= 0; i-- {
				rawValue |= uint64(attr.VendorBytes[i]) << uint64(i*8)
			}
		case "raw48":
			// Big-endian 48-bit number
			for i := 5; i >= 0; i-- {
				rawValue |= uint64(attr.VendorBytes[i]) << uint64(i*8)
			}
		case "tempminmax":
			rawValue = uint64(attr.VendorBytes[0])
		}

		fmt.Printf("%3d %-24s %#04x   %03d   %03d   %03d      %-12d  %v (%s)\n",
			attr.Id, attrconv.Name, attr.Flags, attr.Value, attr.Worst, attr.Reserved,
			rawValue, attr.VendorBytes, attrconv.Conv)
	}

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
