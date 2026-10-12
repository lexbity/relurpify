//go:build linux

package runtime

import (
	"bytes"
	"os"
	"strconv"
	"syscall"
)

// processAlive confirms a PID is live AND is one of ours: on Linux the
// signal-0 probe is narrowed by a /proc/<pid>/cmdline check — a recycled PID
// belonging to an unrelated process does not attach (D7). A live foreign
// process (EPERM on signal-0) is "not confirmably alive", which is the
// honest answer for an operator-facing probe.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil || len(raw) == 0 {
		// Kernel threads have an empty cmdline; a vanished PID is not ours.
		return false
	}
	// The runner binary is `relurpify-runner`; matching the bare substring
	// "relurpify" would also confirm any unrelated process whose path merely
	// contains the project name (a checkout dir, a build-cache path), which is
	// exactly the recycled-PID class this narrowing exists to reject.
	return bytes.Contains(raw, []byte("relurpify-runner"))
}
