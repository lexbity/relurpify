package native_test

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/platform/tools/native"
)

// TestAllConstructorsMatchesRegistry asserts that the explicit constructor map
// returned by AllConstructors covers exactly the set of native keys that the
// init()-based registration exposes.
func TestAllConstructorsMatchesRegistry(t *testing.T) {
	all := native.AllConstructors()
	require.NotEmpty(t, all)

	registered := ports.NativeKeys()
	sort.Strings(registered)

	fromAll := make([]string, 0, len(all))
	for k := range all {
		fromAll = append(fromAll, k)
	}
	sort.Strings(fromAll)

	require.Equal(t, registered, fromAll,
		"AllConstructors must expose the same keys as the global native registry")
}

// TestAllConstructorsBuildWorkingTools asserts every constructor yields a
// fully wired tool when given a workspace base path.
func TestAllConstructorsBuildWorkingTools(t *testing.T) {
	all := native.AllConstructors()
	require.NotEmpty(t, all)

	for key, ctor := range all {
		tool := ctor(t.TempDir())
		require.NotNil(t, tool, "constructor %q returned nil", key)
		require.NotEmpty(t, tool.Name(), "tool from %q has empty name", key)
		require.NotEmpty(t, tool.Description(), "tool from %q has empty description", key)
		require.NotEmpty(t, tool.Category(), "tool from %q has empty category", key)
		require.NotNil(t, tool.Parameters(), "tool from %q has nil parameters", key)
		require.NotNil(t, tool.Permissions().Permissions, "tool from %q has nil permissions", key)
		require.NotEmpty(t, tool.Tags(), "tool from %q has empty tags", key)
	}
}
