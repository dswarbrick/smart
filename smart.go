/*
 * Pure Go SMART library
 *
 * Copyright 2017 Daniel Swarbrick
 */

package smart

import (
	"fmt"
	"path/filepath"
)

func ReadSMART(device string) error {
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
