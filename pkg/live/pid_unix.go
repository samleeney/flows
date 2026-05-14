//go:build !windows

package live

import "syscall"

// ProcessExists reports whether a process with the given PID is alive.
func ProcessExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	// EPERM means the process exists but we cannot signal it.
	if err == syscall.EPERM {
		return true
	}
	return false
}
