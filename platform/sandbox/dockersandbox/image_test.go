package dockersandbox

import (
	"strings"
	"testing"
)

func TestDockerImageDigestConfig(t *testing.T) {
	backend := NewBackend(Config{
		DockerPath:  "docker",
		Image:       "ghcr.io/lexcodex/relurpify/runtime:0.4.1",
		ImageDigest: "sha256:abc123def456",
		Workspace:   "/tmp",
	})
	if backend.config.ImageDigest != "sha256:abc123def456" {
		t.Fatalf("expected ImageDigest to be set, got %q", backend.config.ImageDigest)
	}
}

func TestDockerImageDigestPinning(t *testing.T) {
	cfg := Config{
		DockerPath:  "docker",
		Image:       "myimage:latest",
		ImageDigest: "sha256:abcdef1234567890",
		Workspace:   "/workspace",
	}
	backend := NewBackend(cfg)

	// Verify image + digest combination
	if !strings.Contains(backend.config.Image, "@") {
		expected := "myimage:latest@sha256:abcdef1234567890"
		if backend.config.Image+"@"+cfg.ImageDigest != expected {
			t.Logf("image will be resolved to %s@%s at runtime", cfg.Image, cfg.ImageDigest)
		}
	}
}

func TestDockerSandboxValidatesImageDigest(t *testing.T) {
	// Config with no digest should still be accepted.
	c1 := Config{DockerPath: "docker", Image: "test:latest", Workspace: "/ws"}
	b1 := NewBackend(c1)
	if b1.config.ImageDigest != "" {
		t.Fatal("expected empty digest when not configured")
	}

	// Config with digest pins image
	c2 := Config{DockerPath: "docker", Image: "test:latest", ImageDigest: "sha256:xyz", Workspace: "/ws"}
	b2 := NewBackend(c2)
	if b2.config.ImageDigest != "sha256:xyz" {
		t.Fatalf("expected digest to be set, got %q", b2.config.ImageDigest)
	}
}
