package cfgload

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

func TestPathsGovernanceRoots(t *testing.T) {
	paths := New("/workspace")
	require.Equal(t, filepath.Join("/workspace", DirName), paths.ConfigRoot())
	require.Contains(t, paths.GovernanceRoots("/tmp/extra"), filepath.Join("/workspace", DirName))
	require.Contains(t, paths.GovernanceRoots("/tmp/extra"), filepath.Join("/workspace", DirName, "agents"))
}

func TestLoadAgentManifestSnapshotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	manifest := &AgentManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentManifest",
		Metadata: ManifestMetadata{
			Name: "demo",
		},
		Spec: ManifestSpec{
			Image:   "ghcr.io/lexcodex/relurpify/runtime:0.4.1",
			Runtime: "gvisor",
			Policy: &ManifestPolicySpec{
				Permissions: permissionSetForManifestTest(),
				Resources: ResourceSpec{
					Limits: ResourceLimit{
						CPU:    "1",
						Memory: "1Gi",
						DiskIO: "1MBps",
					},
				},
				Security: SecuritySpec{
					RunAsUser:       1000,
					ReadOnlyRoot:    true,
					NoNewPrivileges: true,
				},
				Audit: AuditSpec{
					Level:         "verbose",
					RetentionDays: 7,
				},
			},
		},
	}

	require.NoError(t, SaveAgentManifest(path, manifest))

	snapshot, err := LoadAgentManifestSnapshot(path)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.Equal(t, path, snapshot.SourcePath)
	require.Equal(t, "demo", snapshot.Manifest.Metadata.Name)
	require.Equal(t, [32]byte(sha256SumForTest(path, t)), snapshot.Fingerprint)
}

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

func TestLoadSkillListUsesCanonicalPath(t *testing.T) {
	root := t.TempDir()
	skillsDir := filepath.Join(root, "relurpify_cfg", "skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	path := filepath.Join(skillsDir, "demo.skill.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`schema: relurpify/skill/v1
apiVersion: relurpify/v1alpha1
kind: SkillManifest
metadata:
  name: demo
spec:
  requires:
    bins: [git]
`), 0o644))

	skills := LoadSkillList(root, []string{"demo", "missing"})
	require.Len(t, skills, 1)
	require.Equal(t, "demo", skills[0].Metadata.Name)
}

func permissionSetForManifestTest() contracts.PermissionSet {
	return contracts.PermissionSet{
		FileSystem: []contracts.FileSystemPermission{{
			Action: contracts.FileSystemRead,
			Path:   "${workspace}/**",
		}},
		Executables: []contracts.ExecutablePermission{{
			Binary: "bash",
			Args:   []string{"-lc"},
		}},
		Network: []contracts.NetworkPermission{{
			Direction: "egress",
			Host:      "localhost",
			Port:      11434,
			Protocol:  "tcp",
		}},
	}
}

func sha256SumForTest(path string, t *testing.T) [32]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return sha256.Sum256(data)
}
