// Copyright 2017 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

// NVMe admin commands.

package smart

import (
	"encoding/binary"
	"fmt"
	"syscall"
	"unsafe"
)

const (
	NVME_ADMIN_IDENTIFY = 0x06
)

var (
	NVME_IOCTL_ADMIN_CMD = _iowr('N', 0x41, unsafe.Sizeof(nvmePassthruCommand{}))
)

// Defined in <linux/nvme_ioctl.h>
type nvmePassthruCommand struct {
	opcode       uint8
	flags        uint8
	rsvd1        uint16
	nsid         uint32
	cdw2         uint32
	cdw3         uint32
	metadata     uint64
	addr         uint64
	metadata_len uint32
	data_len     uint32
	cdw10        uint32
	cdw11        uint32
	cdw12        uint32
	cdw13        uint32
	cdw14        uint32
	cdw15        uint32
	timeout_ms   uint32
	result       uint32
} // 72 bytes

// WIP, highly likely to change
func OpenNVMe(dev string) error {
	fd, err := syscall.Open(dev, syscall.O_RDWR, 0600)
	if err != nil {
		return err
	}

	defer syscall.Close(fd)

	buf := make([]byte, 4096)

	cmd := nvmePassthruCommand{
		opcode:   NVME_ADMIN_IDENTIFY,
		nsid:     0, // Namespace 0, since we are identifying the controller
		addr:     uint64(uintptr(unsafe.Pointer(&buf[0]))),
		data_len: uint32(len(buf)),
		cdw10:    1, // Identify controller
	}

	fmt.Printf("unsafe.Sizeof(cmd): %d\n", unsafe.Sizeof(cmd))
	fmt.Printf("binary.Size(cmd): %d\n", binary.Size(cmd))

	if err := ioctl(uintptr(fd), NVME_IOCTL_ADMIN_CMD, uintptr(unsafe.Pointer(&cmd))); err != nil {
		return err
	}

	fmt.Printf("NVMe call: opcode=%#02x, size=%#04x, nsid=%#08x, cdw10=%#08x\n",
		cmd.opcode, cmd.data_len, cmd.nsid, cmd.cdw10)
	fmt.Printf("%#v\n", buf)

	return nil
}
