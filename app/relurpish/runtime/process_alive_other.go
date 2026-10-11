//go:build !linux

package runtime

import "syscall"

// processAlive is the signal-0 probe on non-Linux platforms; the
// /proc/<pid>/cmdline narrowing is Linux-only by design (documented
// platform asymmetry, D7).
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}
