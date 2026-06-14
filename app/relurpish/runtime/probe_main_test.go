package runtime

import (
	"os"
	"testing"
)

// TestMain disables the external runsc/docker/containerd version probes for the
// whole package test run. Those probes shell out to `docker info`, `runsc
// --version`, etc., which contact the Docker daemon and can trigger desktop
// privilege-elevation prompts during a plain `go test ./...`. Tests that need
// to exercise the real probe path can temporarily set sandboxProbeDisabled
// back to false.
func TestMain(m *testing.M) {
	sandboxProbeDisabled = true
	os.Exit(m.Run())
}
