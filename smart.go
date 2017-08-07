// Copyright 2017 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

// Package smart is a pure Go SMART library.
//
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
func (sa *smartAttr) decodeVendorBytes(conv string) uint64 {
	var (
		byteOrder string
		r         uint64
	)

	// Default byte orders if not otherwise specified in drivedb
	// TODO: Handle temperature formats (device-specific)
	switch conv {
	case "raw64", "hex64":
		byteOrder = "543210wv"
	case "raw56", "hex56", "raw24/raw32", "msec24hour32":
		byteOrder = "r543210"
	default:
		byteOrder = "543210"
	}

	// Pick bytes from smartAttr in order specified by byteOrder
	for _, i := range byteOrder {
		var b byte

		switch i {
		case '0', '1', '2', '3', '4', '5':
			b = sa.VendorBytes[i-48]
		case 'r':
			b = sa.Reserved
		case 'v':
			b = sa.Value
		case 'w':
			b = sa.Worst
		default:
			b = 0
		}

		r <<= 8
		r |= uint64(b)
	}

	return r
}

func formatRawValue(v uint64, conv string) (s string) {
	var (
		raw  [6]uint8
		word [3]uint16
	)

	// Split into bytes
	for i := 0; i < 6; i++ {
		raw[i] = uint8(v >> uint(i*8))
	}

	// Split into words
	for i := 0; i < 3; i++ {
		word[i] = uint16(v >> uint(i*16))
	}

	switch conv {
	case "raw8":
		s = fmt.Sprintf("%d %d %d %d %d %d",
			raw[5], raw[4], raw[3], raw[2], raw[1], raw[0])
	case "raw16":
		s = fmt.Sprintf("%d %d %d", word[2], word[1], word[0])
	case "raw48", "raw56", "raw64":
		s = fmt.Sprintf("%d", v)
	case "hex48":
		s = fmt.Sprintf("%#012x", v)
	case "hex56":
		s = fmt.Sprintf("%#014x", v)
	case "hex64":
		s = fmt.Sprintf("%#016x", v)
	case "raw16(raw16)":
		s = fmt.Sprintf("%d", word[0])
		if (word[1] != 0) || (word[2] != 0) {
			s += fmt.Sprintf(" (%d %d)", word[2], word[1])
		}
	case "raw16(avg16)":
		s = fmt.Sprintf("%d", word[0])
		if word[1] != 0 {
			s += fmt.Sprintf(" (Average %d)", word[1])
		}
	case "raw24(raw8)":
		s = fmt.Sprintf("%d", v&0x00ffffff)
		if (raw[3] != 0) || (raw[4] != 0) || (raw[5] != 0) {
			s += fmt.Sprintf(" (%d %d %d)", raw[5], raw[4], raw[3])
		}
	case "raw24/raw24":
		s = fmt.Sprintf("%d/%d", v>>24, v&0x00ffffff)
	case "raw24/raw32":
		s = fmt.Sprintf("%d/%d", v>>32, v&0xffffffff)
	case "min2hour":
		// minutes
		minutes := int64(word[0]) + int64(word[1])<<16
		s = fmt.Sprintf("%dh+%02dm", minutes/60, minutes%60)
		if word[2] != 0 {
			s += fmt.Sprintf(" (%d)", word[2])
		}
	case "sec2hour":
		// seconds
		hours := v / 3600
		minutes := (v % 3600) / 60
		seconds := v % 60
		s = fmt.Sprintf("%dh+%02dm+%02ds", hours, minutes, seconds)
	case "halfmin2hour":
		// 30-second counter
		hours := v / 120
		minutes := (v % 120) / 2
		s = fmt.Sprintf("%dh+%02dm", hours, minutes)
	case "msec24hour32":
		// hours + milliseconds
		hours := v & 0xffffffff
		milliseconds := v >> 32
		seconds := milliseconds / 1000
		s = fmt.Sprintf("%dh+%02dm+%02d.%03ds",
			hours, seconds/60, seconds%60, milliseconds)
	case "tempminmax":
		s = "not implemented"
	case "temp10x":
		// ten times temperature in Celsius
		s = fmt.Sprintf("%d.%d", word[0]/10, word[0]%10)
	default:
		s = "?"
	}

	return s
}

func printSMART(smart smartPage, drive driveModel) {
	fmt.Printf("\nSMART structure version: %d\n", smart.Version)
	fmt.Printf("ID# ATTRIBUTE_NAME           FLAG     VALUE WORST RESERVED TYPE     UPDATED RAW_VALUE           VENDOR_BYTES\n")

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
		if attr.Flags&0x0001 != 0 {
			attrType = "Pre-fail"
		} else {
			attrType = "Old_age"
		}

		// Online data collection bit
		if attr.Flags&0x0002 != 0 {
			attrUpdated = "Always"
		} else {
			attrUpdated = "Offline"
		}

		fmt.Printf("%3d %-24s %#04x   %03d   %03d   %03d      %-8s %-7s %-18s  %v (%s)\n",
			attr.Id, conv.Name, attr.Flags, attr.Value, attr.Worst, attr.Reserved, attrType,
			attrUpdated, formatRawValue(rawValue, conv.Conv), attr.VendorBytes, conv.Conv)
	}
}
