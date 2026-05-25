package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)



func TestLoadAgentManifestRequiresSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: demo
spec:
  image: demo
  runtime: gvisor
  permissions:
    filesystem: []
`), 0o644))

	_, err := LoadAgentManifest(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing schema declaration")
}

func TestLoadAgentManifestRejectsAnchors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`schema: relurpify/agent/v1
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: demo
spec:
  image: demo
  runtime: gvisor
  permissions:
    filesystem: &fs []
  resources:
    limits:
      cpu: 1
      memory: 1Gi
      disk_io: 1MBps
  security:
    run_as_user: 1000
    read_only_root: true
    no_new_privileges: true
  audit:
    level: verbose
    retention_days: 7
  agent:
    model:
      provider: ollama
      name: demo
    mode: primary
    allowed_capabilities: *fs
`), 0o644))

	_, err := LoadAgentManifest(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "yaml anchor or alias not allowed")
}

func TestLoadSkillManifestRequiresSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`apiVersion: relurpify/v1alpha1
kind: SkillManifest
metadata:
  name: demo
spec:
  requires:
    bins: [git]
`), 0o644))

	_, err := LoadSkillManifest(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing schema declaration")
}

func TestLoadSkillManifestRejectsUnknownSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`schema: relurpify/unknown/v1
apiVersion: relurpify/v1alpha1
kind: SkillManifest
metadata:
  name: demo
spec:
  requires:
    bins: [git]
`), 0o644))

	_, err := LoadSkillManifest(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown schema")
}

func TestLoadSkillManifestRejectsWorkflowFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`schema: relurpify/skill/v1
apiVersion: relurpify/v1alpha1
kind: SkillManifest
metadata:
  name: demo
spec:
  requires:
    bins: [git]
  planning:
    require_verification_step: true
`), 0o644))

	_, err := LoadSkillManifest(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "planning")
}

func TestLoadAgentManifestRejectsUnsupportedVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`schema: relurpify/agent/v2
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: demo
spec:
  image: demo
  runtime: gvisor
  permissions:
    filesystem: []
  resources:
    limits:
      cpu: 1
      memory: 1Gi
      disk_io: 1MBps
  security:
    run_as_user: 1000
    read_only_root: true
    no_new_privileges: true
  audit:
    level: verbose
    retention_days: 7
`), 0o644))

	_, err := LoadAgentManifest(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported schema version")
}
