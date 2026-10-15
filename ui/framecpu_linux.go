//go:build linux

package ui

import (
	"syscall"
	"time"
	"unsafe"
)

// CLOCK_THREAD_CPUTIME_ID excludes time waiting for the compositor/GPU.
func frameThreadCPU() time.Duration {
	var ts syscall.Timespec
	_, _, err := syscall.RawSyscall(syscall.SYS_CLOCK_GETTIME, 3, uintptr(unsafe.Pointer(&ts)), 0)
	if err != 0 {
		return 0
	}
	return time.Duration(ts.Sec)*time.Second + time.Duration(ts.Nsec)
}
