package agents

import (
	"os"
	"path/filepath"
	"testing"

	frameworkconfig "github.com/lexcodex/relurpify/framework/config"
	"github.com/stretchr/testify/require"
)

// TestRegistryLoad verifies that manifests are discovered from configured paths.
func TestRegistryLoad(t *testing.T) {
	dir := t.TempDir()
	projectAgents := filepath.Join(frameworkconfig.New(dir).ConfigRoot(), "agents")
	require.NoError(t, os.MkdirAll(projectAgents, 0o755))

	content := `
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: demo
  version: "1.0.0"
  description: demo agent
spec:
  image: ghcr.io/relurpify/runtime:latest
  runtime: gvisor
  permissions:
    filesystem:
      - action: fs:read
        path: ` + filepath.ToSlash(filepath.Join(dir, "**")) + `
        justification: Read workspace
    executables:
      - binary: bash
        args: ["-c"]
    network:
      - direction: egress
        protocol: tcp
        host: localhost
        port: 11434
  resources:
    limits:
      cpu: "1"
      memory: 1Gi
      disk_io: 100MBps
  security:
    run_as_user: 1000
    read_only_root: false
    no_new_privileges: true
  audit:
    level: verbose
    retention_days: 1
  agent:
    mode: primary
    version: "1.0.0"
    prompt: "Be helpful"
    model:
      provider: ollama
      name: codellama:13b
      temperature: 0.1
      max_tokens: 1024
    allowed_capabilities:
      - name: file_read
        kind: tool
      - name: file_write
        kind: tool
      - name: file_edit
        kind: tool
      - name: file_list
        kind: tool
      - name: file_search
        kind: tool
      - name: search_grep
        kind: tool
    bash_permissions:
      allow_patterns: []
      deny_patterns: ["rm -rf*"]
      default: ask
    file_permissions:
      write:
        allow_patterns: ["**/*.go"]
        deny_patterns: []
        default: allow
      edit:
        default: ask
    invocation:
      can_invoke_subagents: true
      allowed_subagents: []
      max_depth: 1
    context:
      max_files: 5
      max_tokens: 2048
      include_git_history: false
      include_dependencies: false
    metadata:
      author: tester
      tags: []
      priority: 1
`
	require.NoError(t, os.WriteFile(filepath.Join(projectAgents, "demo.yaml"), []byte(content), 0o644))

	reg := NewRegistry(RegistryOptions{
		Workspace: dir,
		Paths:     []string{projectAgents},
	})
	require.NoError(t, reg.Load())
	list := reg.List()
	require.Len(t, list, 1)
	require.Equal(t, "demo", list[0].Name)
}

func TestRegistryLoadFilePath(t *testing.T) {
	dir := t.TempDir()
	cfgDir := frameworkconfig.New(dir).ConfigRoot()
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	manifestPath := filepath.Join(cfgDir, "agent.manifest.yaml")

	content := `
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: solo
  version: "1.0.0"
  description: solo agent
spec:
  image: ghcr.io/relurpify/runtime:latest
  runtime: gvisor
  permissions:
    filesystem:
      - action: fs:read
        path: ` + filepath.ToSlash(filepath.Join(dir, "**")) + `
        justification: Read workspace
    executables:
      - binary: bash
        args: ["-c"]
  agent:
    mode: primary
    version: "1.0.0"
    prompt: "Be helpful"
    model:
      provider: ollama
      name: codellama:13b
      temperature: 0.1
      max_tokens: 1024
    allowed_capabilities:
      - name: file_read
        kind: tool
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(content), 0o644))

	reg := NewRegistry(RegistryOptions{
		Workspace: dir,
		Paths:     []string{manifestPath},
	})
	require.NoError(t, reg.Load())
	list := reg.List()
	require.Len(t, list, 1)
	require.Equal(t, "solo", list[0].Name)
}
