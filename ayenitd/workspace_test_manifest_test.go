//go:build integration

package ayenitd_test

import (
	"os"
	"path/filepath"
	"testing"
)

const integrationManifestYAML = `apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: test-agent
  version: 1.0.0
spec:
  image: ghcr.io/lexcodex/relurpify/runtime:latest
  runtime: gvisor
  defaults:
    permissions:
      executables:
        - binary: bash
          args: ["-c", "*"]
        - binary: git
          args: ["*"]
        - binary: go
          args: ["*"]
        - binary: node
          args: ["*"]
        - binary: npm
          args: ["*"]
        - binary: python3
          args: ["*"]
        - binary: cargo
          args: ["*"]
        - binary: rustc
          args: ["*"]
  permissions:
    filesystem:
      - path: /tmp
        action: fs:read
  security:
    no_new_privileges: true
  agent:
    implementation: react
    mode: primary
    model:
      provider: ollama
      name: qwen2.5-coder:14b
    browser:
      enabled: true
`

func writeIntegrationManifest(t *testing.T, workspace string) string {
	t.Helper()
	path := filepath.Join(workspace, "agent.manifest.yaml")
	if err := os.WriteFile(path, []byte(integrationManifestYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
