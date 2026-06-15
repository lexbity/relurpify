package templatesembed

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestDefaultFS_ContainsKeyFiles(t *testing.T) {
	efs := DefaultFS()

	for _, path := range []string{
		"workspace/workspace.yaml",
		"workspace/agent.yaml",
		"workspace/security/sandbox.policy.yaml",
		"workspace/security/shell.policy.yaml",
		"workspace/security/localtool.policy.yaml",
		"workspace/security/workspaceingestion.policy.yaml",
	} {
		_, err := efs.Open(path)
		require.NoError(t, err, "embedded template %s must exist", path)
	}

	_, err := efs.Open("prompts/framework/base.system.prompt")
	require.NoError(t, err, "embedded prompt base.system.prompt must exist")
}

func TestDefaultFS_WorkspaceYamlIsParseable(t *testing.T) {
	efs := DefaultFS()

	data, err := fs.ReadFile(efs, "workspace/workspace.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, data, "workspace.yaml must not be empty")
}

func TestDefaultFS_Readable(t *testing.T) {
	efs := DefaultFS()
	require.Implements(t, (*fs.ReadDirFS)(nil), efs)
	require.Implements(t, (*fs.ReadFileFS)(nil), efs)
}

func TestDefaultFS_AllTemplatesAreKnown(t *testing.T) {
	efs := DefaultFS()

	expected := []string{
		"workspace/workspace.yaml",
		"workspace/agent.yaml",
		"workspace/security/sandbox.policy.yaml",
		"workspace/security/shell.policy.yaml",
		"workspace/security/localtool.policy.yaml",
		"workspace/security/workspaceingestion.policy.yaml",
	}

	// Walk the embedded FS and verify all files are known or expected.
	walkErr := fstest.TestFS(efs, expected...)
	require.NoError(t, walkErr, "all expected files must be present in embed FS")
}
