/*
 * Smartmontools drivedb.h database to .toml format converter
 * Copyright 2017 Daniel Swarbrick
 */

package main

/*
#include "drivedb.h"
*/
import "C"

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type AttrConv struct {
	Conv string
	Name string
}

type DriveModel struct {
	Family        string
	ModelRegex    string
	FirmwareRegex string
	WarningMsg    string
	Presets       map[string]AttrConv
}

type DriveDb struct {
	Drives []DriveModel
}

func main() {
	dest := flag.String("o", "drivedb.toml", "Output .toml file")
	flag.Parse()

	if *dest == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	destFile, err := os.Create(*dest)
	if err != nil {
		fmt.Printf("Cannot create output: %v\n", err)
		os.Exit(1)
	}

	defer destFile.Close()

	drivedb := DriveDb{}
	t0 := time.Now()

	for _, d := range C.builtin_knowndrives {
		dm := DriveModel{}
		dm.Family = C.GoString(d.modelfamily)
		dm.ModelRegex = C.GoString(d.modelregexp)
		dm.FirmwareRegex = C.GoString(d.firmwareregexp)
		dm.WarningMsg = C.GoString(d.warningmsg)
		dm.Presets = make(map[string]AttrConv)

		/* Split presets params up so we can parse them */
		tokens := strings.Split(C.GoString(d.presets), " ")

		for t := 0; t < len(tokens); t += 2 {
			/* How to parse vendor bytes */
			if tokens[t] == "-v" {
				attrs := strings.Split(tokens[t+1], ",")

				if len(attrs) >= 3 {
					dm.Presets[attrs[0]] = AttrConv{Conv: attrs[1], Name: attrs[2]}
				} else {
					dm.Presets[attrs[0]] = AttrConv{Conv: attrs[1]}
				}
			}
		}

		drivedb.Drives = append(drivedb.Drives, dm)
	}

	fmt.Printf("Parsed drivedb.h in %v - %d entries\n", time.Since(t0), len(drivedb.Drives))
	enc := toml.NewEncoder(destFile)

	if err := enc.Encode(drivedb); err != nil {
		fmt.Printf("Error encoding toml: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully wrote output to %s\n", *dest)
}
