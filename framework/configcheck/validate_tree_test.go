package configcheck

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateWorkspaceTreeRepoRoot(t *testing.T) {
	report := ValidateWorkspaceTree(repoRoot(t))
	require.False(t, report.HasErrors(), report.Error())
}

func repoRoot(t *testing.T) string {
	t.Helper()
	return filepath.Clean(filepath.Join("..", ".."))
}
