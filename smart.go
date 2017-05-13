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
	Value       uint8
	Worst       uint8
	VendorBytes [6]byte
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

func printSMART(smart smartPage, drive driveModel) {
	fmt.Printf("\nSMART structure version: %d\n", smart.Version)
	fmt.Printf("ID# ATTRIBUTE_NAME           FLAG     VALUE WORST RESERVED RAW_VALUE     VENDOR_BYTES\n")

	for _, attr := range smart.Attrs {
		var rawValue uint64

		if attr.Id == 0 {
			break
		}

		attrconv := drive.Presets[strconv.Itoa(int(attr.Id))]

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
}
