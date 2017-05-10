/*
 * Pure Go SMART library
 * Copyright 2017 Daniel Swarbrick
 *
 * Avago MegaRAID ioctl functions
 * TODO:
 * - Device Scan:
 *   - Walk /sys/class/scsi_host/ directory
 *   - "host%d" symlinks enumerate hosts
 *   - "host%d/proc_name" should contain the value "megaraid_sas"
 */

package smart

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"
)

type MegasasIoctl struct {
	DeviceMajor int
	fd          int
}

// MakeDev returns the device ID for the specified major and minor numbers, equivalent to
// makedev(3). Based on gnu_dev_makedev macro, may be platform dependent!
func MakeDev(major, minor uint) uint {
	return (minor & 0xff) | ((major & 0xfff) << 8) |
		((minor &^ 0xff) << 12) | ((major &^ 0xfff) << 32)
}

// CreateMegasasIoctl determines the device ID for the MegaRAID SAS ioctl device, creates it
// if necessary, and returns a MegasasIoctl struct to manage the device.
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

		syscall.Mknod("/dev/megaraid_sas_ioctl_node", syscall.S_IFCHR, int(MakeDev(uint(m.DeviceMajor), 0)))
	} else {
		return m, err
	}

	m.fd, err = syscall.Open("/dev/megaraid_sas_ioctl_node", syscall.O_RDWR, 0600)

	if err != nil {
		return m, err
	}

	return m, nil
}

// Close closes the file descriptor of the MegasasIoctl instance
func (m *MegasasIoctl) Close() {
	syscall.Close(m.fd)
}

func OpenMegasasIoctl() error {
	m, _ := CreateMegasasIoctl()
	fmt.Printf("%#v\n", m)

	defer m.Close()

	return nil
}
