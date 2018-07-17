// Copyright 2017-18 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

// Smartmontools drivedb.h database to YAML format converter.
//
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/scanner"
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
	input, err := os.Open("drivedb.h")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot open source file: %v\n", err)
		os.Exit(1)
	}

	defer input.Close()

	var (
		s       scanner.Scanner
		prev    rune
		idx     int
		items   = make([]string, 5)
		drivedb DriveDb
	)

	s.Init(input)
	t0 := time.Now()

	for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
		if (prev == '{' || prev == ',') && tok == scanner.String {
			items[idx] = strings.Trim(s.TokenText(), `"`)
		} else if prev == scanner.String && tok == ',' {
			idx++
		} else if prev == scanner.String && tok == scanner.String {
			items[idx] += strings.Trim(s.TokenText(), `"`)
		} else if tok == '}' {
			dm := DriveModel{Presets: make(map[string]AttrConv)}

			if tmp, err := strconv.Unquote(`"` + items[0] + `"`); err == nil {
				dm.Family = tmp
			}

			if tmp, err := strconv.Unquote(`"` + items[1] + `"`); err == nil {
				dm.ModelRegex = tmp
			}

			if tmp, err := strconv.Unquote(`"` + items[2] + `"`); err == nil {
				dm.FirmwareRegex = tmp
			}

			if tmp, err := strconv.Unquote(`"` + items[3] + `"`); err == nil {
				dm.WarningMsg = tmp
			}

			// Split presets params so we can parse them.
			attrTokens := strings.Split(items[4], " ")

			for t := 0; t < len(attrTokens); t += 2 {
				if attrTokens[t] == "-v" {
					attrs := strings.Split(attrTokens[t+1], ",")

					if len(attrs) >= 3 {
						dm.Presets[attrs[0]] = AttrConv{Conv: attrs[1], Name: attrs[2]}
					} else {
						dm.Presets[attrs[0]] = AttrConv{Conv: attrs[1]}
					}
				}
			}

			drivedb.Drives = append(drivedb.Drives, dm)

			items = make([]string, 5)
			idx = 0
		}

		prev = tok
	}

	dest := flag.String("o", "drivedb.yml", "Output .yml file")
	flag.Parse()

	if *dest == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	destFile, err := os.Create(*dest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot create output: %v\n", err)
		os.Exit(1)
	}

	defer destFile.Close()

	fmt.Printf("Parsed drivedb.h in %v - %d entries\n", time.Since(t0), len(drivedb.Drives))
	enc := yaml.NewEncoder(destFile)

	if err := enc.Encode(drivedb); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding yaml: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully wrote output to %s\n", *dest)
}
