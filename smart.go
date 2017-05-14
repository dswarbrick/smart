/*
 * Pure Go SMART library
 * Copyright 2017 Daniel Swarbrick
 */

package smart

import (
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unsafe"

	"github.com/BurntSushi/toml"
)

// SMART attribute conversion rule
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

// Individual SMART attribute (12 bytes)
type smartAttr struct {
	Id          uint8
	Flags       uint16
	Value       uint8   // normalised value
	Worst       uint8   // worst value
	VendorBytes [6]byte // vendor-specific (and sometimes device-specific) data
	Reserved    uint8
}

// Page of 30 SMART attributes as per ATA spec
type smartPage struct {
	Version uint16
	Attrs   [30]smartAttr
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
	var model driveModel

	for _, d := range db.Drives {
		// Skip placeholder entry
		if strings.HasPrefix(d.Family, "$Id") {
			continue
		}

		if d.Family == "DEFAULT" {
			model = d
			continue
		}

		if d.CompiledRegexp.Match(ident) {
			model.Family = d.Family
			model.ModelRegex = d.ModelRegex
			model.FirmwareRegex = d.FirmwareRegex
			model.WarningMsg = d.WarningMsg
			model.CompiledRegexp = d.CompiledRegexp

			for id, p := range d.Presets {
				if _, exists := model.Presets[id]; exists {
					// Some drives override the conv but don't specify a name, so copy it from default
					if p.Name == "" {
						p.Name = model.Presets[id].Name
					}
				}
				model.Presets[id] = attrConv{Name: p.Name, Conv: p.Conv}
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

// decodeVendorBytes decodes the six-byte vendor byte array based on the conversion rule passed as
// conv. The conversion may also include the reserved byte, normalised value or worst value byte.
func (sa *smartAttr) decodeVendorBytes(conv string) (r uint64) {
	vb := sa.VendorBytes

	// TODO: Complete other attr conversion, honour specified byte order
	switch conv {
	case "raw16(raw16)", "raw16(avg16)":
		r = uint64(vb[0]) | uint64(vb[1])<<8
	case "raw24(raw8)", "raw24/raw24", "raw24/raw32":
		r = uint64(vb[0]) | uint64(vb[1])<<8 | uint64(vb[2])<<16
	case "raw48":
		r = uint64(vb[0]) | uint64(vb[1])<<8 | uint64(vb[2])<<16 |
			uint64(vb[3])<<24 | uint64(vb[4])<<32 | uint64(vb[5])<<40
	case "raw56":
		r = uint64(vb[0]) | uint64(vb[1])<<8 | uint64(vb[2])<<16 | uint64(vb[3])<<24 |
			uint64(vb[4])<<32 | uint64(vb[5])<<40 | uint64(sa.Reserved)<<48
	case "tempminmax":
		// This is device specific!
		r = uint64(vb[0])
	}

	return r
}

func printSMART(smart smartPage, drive driveModel) {
	fmt.Printf("\nSMART structure version: %d\n", smart.Version)
	fmt.Printf("ID# ATTRIBUTE_NAME           FLAG     VALUE WORST RESERVED TYPE     UPDATED RAW_VALUE     VENDOR_BYTES\n")

	for _, attr := range smart.Attrs {
		var (
			rawValue              uint64
			conv                  attrConv
			attrType, attrUpdated string
		)

		if attr.Id == 0 {
			break
		}

		conv, ok := drive.Presets[strconv.Itoa(int(attr.Id))]
		if ok {
			rawValue = attr.decodeVendorBytes(conv.Conv)
		}

		// Pre-fail / advisory bit
		if attr.Flags&0x0001 > 0 {
			attrType = "Pre-fail"
		} else {
			attrType = "Old_age"
		}

		// Online data collection bit
		if attr.Flags&0x0002 > 0 {
			attrUpdated = "Always"
		} else {
			attrUpdated = "Offline"
		}

		fmt.Printf("%3d %-24s %#04x   %03d   %03d   %03d      %-8s %-7s %-12d  %v (%s)\n",
			attr.Id, conv.Name, attr.Flags, attr.Value, attr.Worst, attr.Reserved, attrType,
			attrUpdated, rawValue, attr.VendorBytes, conv.Conv)
	}
}
