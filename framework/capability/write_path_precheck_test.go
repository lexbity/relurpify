package capability

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestWritePathPrecheckAllowsMatchingPath(t *testing.T) {
	err := (WritePathPrecheck{Globs: []string{"**/*.md"}}).Check(core.CapabilityDescriptor{
		EffectClasses: []core.EffectClass{core.EffectClassFilesystemMutation},
	}, map[string]any{"path": "docs/api.md"})

	require.NoError(t, err)
}

func TestWritePathPrecheckBlocksNonMatchingPath(t *testing.T) {
	err := (WritePathPrecheck{Globs: []string{"**/*.md"}}).Check(core.CapabilityDescriptor{
		EffectClasses: []core.EffectClass{core.EffectClassFilesystemMutation},
	}, map[string]any{"path": "main.go"})

	require.Error(t, err)
	require.Contains(t, err.Error(), `write to "main.go" blocked`)
}

func TestWritePathPrecheckNoOpWhenGlobsUnset(t *testing.T) {
	err := (WritePathPrecheck{}).Check(core.CapabilityDescriptor{
		EffectClasses: []core.EffectClass{core.EffectClassFilesystemMutation},
	}, map[string]any{"path": "main.go"})

	require.NoError(t, err)
}

func TestWritePathPrecheckNoOpForNonWriteCapability(t *testing.T) {
	err := (WritePathPrecheck{Globs: []string{"**/*.md"}}).Check(core.CapabilityDescriptor{
		EffectClasses: []core.EffectClass{core.EffectClassExternalState},
	}, map[string]any{"path": "main.go"})

	require.NoError(t, err)
}

func TestWritePathPrecheckFailsClosedWithoutPathArg(t *testing.T) {
	err := (WritePathPrecheck{Globs: []string{"**/*.md"}}).Check(core.CapabilityDescriptor{
		EffectClasses: []core.EffectClass{core.EffectClassFilesystemMutation},
	}, map[string]any{"content": "hello"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot determine target path")
}

func TestExtractPathArgSupportsStandardKeys(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{name: "path", args: map[string]any{"path": "a.md"}},
		{name: "file_path", args: map[string]any{"file_path": "a.md"}},
		{name: "target", args: map[string]any{"target": "a.md"}},
		{name: "filename", args: map[string]any{"filename": "a.md"}},
		{name: "dest", args: map[string]any{"dest": "a.md"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, ok := extractPathArg(tc.args)
			require.True(t, ok)
			require.Equal(t, "a.md", path)
		})
	}
}
