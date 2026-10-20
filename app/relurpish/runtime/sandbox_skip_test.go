package runtime

import (
	"os/exec"
	"testing"
)

// skipWithoutSandboxBackend skips the test when neither runsc nor docker is
// available on the host. CI environments typically have neither, so tests
// that require a verified sandbox backend must skip rather than fail.
func skipWithoutSandboxBackend(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("runsc"); err == nil {
		return
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return
	}
	t.Skip("sandbox backend (runsc or docker) not available")
}
