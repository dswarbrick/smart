/*
 * Go SMART library smartctl reference implementation
 * Copyright 2017 Daniel Swarbrick
 */

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/dswarbrick/smart"
)

func scanDevices() {
	for _, device := range smart.ScanDevices() {
		fmt.Printf("%#v\n", device)
	}

	smart.MegaScan()
}

func main() {
	fmt.Println("Go smartctl Reference Implementation")
	fmt.Printf("Built with %s on %s (%s)\n\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)

	device := flag.String("device", "", "SATA device from which to read SMART attributes, e.g., /dev/sda")
	megaraid := flag.String("megaraid", "", "MegaRAID host and device ID from which to read SMART attributes, e.g., megaraid0_23")
	scan := flag.Bool("scan", false, "Scan for drives that support SMART")
	flag.Parse()

	if *device != "" {
		if err := smart.ReadSMART(*device); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	} else if *megaraid != "" {
		var (
			host uint16
			disk uint8
		)

		if _, err := fmt.Sscanf(*megaraid, "megaraid%d_%d", &host, &disk); err != nil {
			fmt.Println("Invalid MegaRAID host / device ID syntax")
			os.Exit(1)
		}

		smart.OpenMegasasIoctl(host, disk)
	} else if *scan {
		scanDevices()
	} else {
		flag.PrintDefaults()
		os.Exit(1)
	}
}
