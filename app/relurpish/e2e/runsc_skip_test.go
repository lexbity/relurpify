package e2e

import (
	"os/exec"
	"testing"
)

// skipWithoutRunsc skips the test when runsc (gVisor) is not available on the
// host. CI environments typically lack runsc, so tests that spawn real
// sandboxed runners must skip rather than fail.
func skipWithoutRunsc(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("runsc"); err != nil {
		t.Skip("runsc not available")
	}
}
