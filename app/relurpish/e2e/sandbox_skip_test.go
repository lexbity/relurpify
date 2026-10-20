package e2e

import (
	"os"
	"os/exec"
	"testing"
)

// skipWithoutSandboxBackend skips the test when neither runsc nor docker is
// available on the host, or when running inside a Docker container (where
// docker may be on PATH but the sandbox runtime is not usable). CI
// environments typically have neither, so tests that require a verified
// sandbox backend must skip rather than fail.
func skipWithoutSandboxBackend(t *testing.T) {
	t.Helper()
	if inDockerContainer() {
		t.Skip("running inside a Docker container; sandbox backend not usable")
	}
	if _, err := exec.LookPath("runsc"); err == nil {
		return
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return
	}
	t.Skip("sandbox backend (runsc or docker) not available")
}

// inDockerContainer reports whether the process is running inside a Docker
// container by checking for the /.dockerenv sentinel file.
func inDockerContainer() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}
