package fs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	capregistry "codeburg.org/lexbit/relurpify/capability/registry"
)

type editTestState struct {
	data   map[string]any
	taskID string
	sessID string
}

func (s *editTestState) GetWorkingValue(key string) (any, bool) {
	v, ok := s.data[key]
	return v, ok
}

func (s *editTestState) SetWorkingValue(key string, value any) {
	if s.data == nil {
		s.data = make(map[string]any)
	}
	s.data[key] = value
}

func (s *editTestState) DeleteWorkingValue(key string) {
	delete(s.data, key)
}

func (s *editTestState) ClearWorkingData() {
	s.data = make(map[string]any)
}

func (s *editTestState) WorkingMemoryKeys() []string {
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	return keys
}

func (s *editTestState) Snapshot() map[string]any {
	out := make(map[string]any, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}

func (s *editTestState) TaskID() string    { return s.taskID }
func (s *editTestState) SessionID() string { return s.sessID }

func TestEditFileTool_ExactReplacement(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), PublicDirMode))
	require.NoError(t, WriteFileSecure(filepath.Join(dir, "docs", "target.txt"), []byte("alpha and alpha\n")))

	tool := &EditFileTool{BasePath: dir}
	result, err := tool.Execute(context.Background(), map[string]any{
		"path":           "docs/target.txt",
		"old_string":     "alpha",
		"new_string":     "beta",
		"expected_count": 2,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	content, err := os.ReadFile(filepath.Join(dir, "docs", "target.txt"))
	require.NoError(t, err)
	assert.Equal(t, "beta and beta\n", string(content))
	require.Equal(t, 2, result.Data["replaced_count"])
}

func TestEditFileTool_CountMismatch_NoWrite(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), PublicDirMode))
	require.NoError(t, WriteFileSecure(filepath.Join(dir, "docs", "target.txt"), []byte("alpha and alpha\n")))

	tool := &EditFileTool{BasePath: dir}
	result, err := tool.Execute(context.Background(), map[string]any{
		"path":       "docs/target.txt",
		"old_string": "alpha",
		"new_string": "beta",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "expected 1 occurrence")

	content, err := os.ReadFile(filepath.Join(dir, "docs", "target.txt"))
	require.NoError(t, err)
	assert.Equal(t, "alpha and alpha\n", string(content))
}

func TestEditFileTool_PathEscape(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target.txt"), []byte("alpha\n"), SecureFileMode))

	tool := &EditFileTool{BasePath: dir}
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":       "../target.txt",
		"old_string": "alpha",
		"new_string": "beta",
	})
	require.ErrorIs(t, err, ErrPathEscapesBase)
	content, err := os.ReadFile(filepath.Join(dir, "target.txt"))
	require.NoError(t, err)
	assert.Equal(t, "alpha\n", string(content))
}

func TestEditFileTool_RollbackViaRegistry(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), PublicDirMode))
	require.NoError(t, WriteFileSecure(filepath.Join(dir, "docs", "target.txt"), []byte("alpha\n")))

	reg := capregistry.NewRegistry()
	tool := &EditFileTool{BasePath: dir}
	require.NoError(t, reg.RegisterLegacyTool(context.Background(), tool))

	result, err := reg.InvokeCapability(context.Background(), &editTestState{taskID: "task", sessID: "sess"}, "file_edit", map[string]any{
		"path":       "docs/target.txt",
		"old_string": "alpha",
		"new_string": "beta",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	token, ok := result.Metadata["rollback_token"].(string)
	require.True(t, ok)
	require.NotEmpty(t, token)

	content, err := os.ReadFile(filepath.Join(dir, "docs", "target.txt"))
	require.NoError(t, err)
	assert.Equal(t, "beta\n", string(content))

	require.NoError(t, reg.RollbackCapability(context.Background(), token))
	content, err = os.ReadFile(filepath.Join(dir, "docs", "target.txt"))
	require.NoError(t, err)
	assert.Equal(t, "alpha\n", string(content))
}

var _ ports.State = (*editTestState)(nil)
