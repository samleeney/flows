//go:build windows

package live

import "syscall"

// ProcessExists reports whether a process with the given PID is alive.
func ProcessExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	syscall.CloseHandle(h)
	return true
}
