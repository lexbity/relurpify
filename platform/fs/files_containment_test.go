package fs

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileToolsRejectEscapePaths(t *testing.T) {
	base := t.TempDir()
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "read",
			run: func(t *testing.T) {
				tool := &ReadFileTool{BasePath: base}
				_, err := tool.Execute(context.Background(), map[string]any{"path": "../etc/passwd"})
				require.Error(t, err)
			},
		},
		{
			name: "write",
			run: func(t *testing.T) {
				tool := &WriteFileTool{BasePath: base}
				_, err := tool.Execute(context.Background(), map[string]any{"path": "../etc/passwd", "content": "x"})
				require.Error(t, err)
			},
		},
		{
			name: "list",
			run: func(t *testing.T) {
				tool := &ListFilesTool{BasePath: base}
				_, err := tool.Execute(context.Background(), map[string]any{"directory": "../etc", "pattern": "*"})
				require.Error(t, err)
			},
		},
		{
			name: "search",
			run: func(t *testing.T) {
				tool := &SearchInFilesTool{BasePath: base}
				_, err := tool.Execute(context.Background(), map[string]any{"directory": "../etc", "pattern": "TODO"})
				require.Error(t, err)
			},
		},
		{
			name: "create",
			run: func(t *testing.T) {
				tool := &CreateFileTool{BasePath: base}
				_, err := tool.Execute(context.Background(), map[string]any{"path": filepath.Join("..", "etc", "passwd"), "content": "x"})
				require.Error(t, err)
			},
		},
		{
			name: "delete",
			run: func(t *testing.T) {
				tool := &DeleteFileTool{BasePath: base}
				_, err := tool.Execute(context.Background(), map[string]any{"path": "../etc/passwd"})
				require.Error(t, err)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}
