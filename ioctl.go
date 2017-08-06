// Copyright 2017 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

// Implementation of Linux kernel ioctl macros (<uapi/asm-generic/ioctl.h>).
// See https://www.kernel.org/doc/Documentation/ioctl/ioctl-number.txt

package smart

import "syscall"

const (
	directionNone  = 0
	directionWrite = 1
	directionRead  = 2

	numberBits    = 8
	typeBits      = 8
	sizeBits      = 14
	directionBits = 2

	numberMask    = (1 << numberBits) - 1
	typeMask      = (1 << typeBits) - 1
	sizeMask      = (1 << sizeBits) - 1
	directionMask = (1 << directionBits) - 1

	numberShift    = 0
	typeShift      = numberShift + numberBits
	sizeShift      = typeShift + typeBits
	directionShift = sizeShift + sizeBits
)

// _ioc calculates the ioctl command for the specified direction, type, number and size
func _ioc(dir, t, nr, size uintptr) uintptr {
	return (dir << directionShift) | (t << typeShift) | (nr << numberShift) | (size << sizeShift)
}

// _ior calculates the ioctl command for a read-ioctl of the specified type, number and size
func _ior(t, nr, size uintptr) uintptr {
	return _ioc(directionRead, t, nr, size)
}

// _iow calculates the ioctl command for a write-ioctl of the specified type, number and size
func _iow(t, nr, size uintptr) uintptr {
	return _ioc(directionWrite, t, nr, size)
}

// _iowr calculates the ioctl command for a read/write-ioctl of the specified type, number and size
func _iowr(t, nr, size uintptr) uintptr {
	return _ioc(directionWrite|directionRead, t, nr, size)
}

// ioctl executes an ioctl command on the specified file descriptor
func ioctl(fd, cmd, ptr uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, cmd, ptr)
	if errno != 0 {
		return errno
	}
	return nil
}
