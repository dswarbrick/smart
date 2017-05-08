/*
 * Pure Go SMART library
 * Copyright 2017 Daniel Swarbrick
 *
 * Implementation of Linux kernel ioctl macros (<uapi/asm-generic/ioctl.h>)
 * See https://www.kernel.org/doc/Documentation/ioctl/ioctl-number.txt
 */

package smart

import "syscall"

// ioctl executes an ioctl command on the specified file descriptor
func ioctl(fd, cmd, ptr uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, cmd, ptr)
	if errno != 0 {
		return errno
	}
	return nil
}
