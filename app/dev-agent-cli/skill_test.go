package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/testsuite/agenttest"
	"github.com/stretchr/testify/require"
)

func TestWriteSkillTestSuiteUsesDerivedWorkspaceTemplate(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, writeSkillTestSuite(root, "demo-skill", "coding", false))

	suite, err := agenttest.LoadSuite(filepath.Join(root, "testsuite.yaml"))
	require.NoError(t, err)
	require.Equal(t, "derived", suite.Spec.Workspace.Strategy)
	require.Equal(t, "default", suite.Spec.Workspace.TemplateProfile)
}

func TestWriteSkillTestSuiteDoesNotOverwriteWithoutForce(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "testsuite.yaml")
	require.NoError(t, os.WriteFile(path, []byte("existing"), 0o644))

	err := writeSkillTestSuite(root, "demo-skill", "coding", false)
	require.Error(t, err)
}
