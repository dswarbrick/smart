// Copyright 2017-18 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

// Smartmontools drivedb.h database to YAML format converter.
//
package main

// #include "drivedb.h"
import "C"

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

type AttrConv struct {
	Conv string
	Name string `yaml:",omitempty"`
}

type DriveModel struct {
	Family        string `yaml:",omitempty"`
	ModelRegex    string
	FirmwareRegex string              `yaml:",omitempty"`
	WarningMsg    string              `yaml:",omitempty"`
	Presets       map[string]AttrConv `yaml:",omitempty"`
}

type DriveDb struct {
	Drives []DriveModel
}

func main() {
	dest := flag.String("o", "drivedb.yml", "Output .yml file")
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
	enc := yaml.NewEncoder(destFile)

	if err := enc.Encode(drivedb); err != nil {
		fmt.Printf("Error encoding yaml: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully wrote output to %s\n", *dest)
}
