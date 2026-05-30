package contracts

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// testNativeTool is a minimal Tool implementation for registry tests.
type testNativeTool struct {
	name  string
	ready bool
}

func (t *testNativeTool) Name() string                          { return t.name }
func (t *testNativeTool) Description() string                   { return "test tool: " + t.name }
func (t *testNativeTool) Category() string                      { return "test" }
func (t *testNativeTool) Parameters() []ToolParameter           { return nil }
func (t *testNativeTool) Tags() []string                        { return nil }
func (t *testNativeTool) Permissions() ToolPermissions          { return ToolPermissions{Permissions: &PermissionSet{}} }
func (t *testNativeTool) IsAvailable(ctx context.Context) bool   { return true }
func (t *testNativeTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	return &ToolResult{Success: true, Data: map[string]interface{}{"name": t.name}}, nil
}

func TestNativeRegisterAndLookup(t *testing.T) {
	ResetNativeRegistry()
	t.Cleanup(ResetNativeRegistry)

	ctor := func(basePath string) Tool {
		return &testNativeTool{name: "search_semantic", ready: true}
	}

	RegisterNative("search_semantic", ctor)

	got, ok := LookupNative("search_semantic")
	require.True(t, ok, "LookupNative should find registered key")
	require.NotNil(t, got)

	tool := got("/tmp/test")
	require.Equal(t, "search_semantic", tool.Name())
}

func TestNativeLookupWithNormalizedVariants(t *testing.T) {
	ResetNativeRegistry()
	t.Cleanup(ResetNativeRegistry)

	RegisterNative("search_semantic", func(basePath string) Tool {
		return &testNativeTool{name: "search_semantic"}
	})

	for _, variant := range []string{
		"search_semantic",
		"Search-Semantic",
		"  SEARCH.SEMANTIC  ",
		"search/semantic",
		"search  semantic",
	} {
		got, ok := LookupNative(variant)
		require.True(t, ok, "LookupNative(%q) should succeed", variant)
		require.NotNil(t, got)
	}
}

func TestNativeLookupMissingKey(t *testing.T) {
	ResetNativeRegistry()
	t.Cleanup(ResetNativeRegistry)

	_, ok := LookupNative("does_not_exist")
	require.False(t, ok, "LookupNative for unregistered key should return false")
}

func TestNativeLookupEmptyKey(t *testing.T) {
	_, ok := LookupNative("")
	require.False(t, ok, "LookupNative for empty key should return false")

	_, ok = LookupNative("   ")
	require.False(t, ok, "LookupNative for whitespace-only key should return false")
}

func TestNativeRegisterDuplicatePanics(t *testing.T) {
	ResetNativeRegistry()
	t.Cleanup(ResetNativeRegistry)

	ctor := func(basePath string) Tool {
		return &testNativeTool{name: "dup"}
	}
	RegisterNative("tool_one", ctor)

	require.Panics(t, func() {
		RegisterNative("tool_one", ctor)
	}, "RegisterNative should panic on duplicate key")
}

func TestNativeRegisterDoubleNormalizationPanics(t *testing.T) {
	ResetNativeRegistry()
	t.Cleanup(ResetNativeRegistry)

	ctor := func(basePath string) Tool {
		return &testNativeTool{name: "dup"}
	}
	RegisterNative("tool-two", ctor)

	require.Panics(t, func() {
		RegisterNative("tool.two", ctor)
	}, "RegisterNative should panic on normalization-collision (tool-two vs tool.two)")
}

func TestNativeRegisterEmptyKeyPanics(t *testing.T) {
	ResetNativeRegistry()
	t.Cleanup(ResetNativeRegistry)

	ctor := func(basePath string) Tool {
		return &testNativeTool{name: "empty"}
	}
	require.Panics(t, func() {
		RegisterNative("", ctor)
	}, "RegisterNative should panic on empty key")
}

func TestNativeRegisterNilConstructorPanics(t *testing.T) {
	ResetNativeRegistry()
	t.Cleanup(ResetNativeRegistry)

	require.Panics(t, func() {
		RegisterNative("nil_ctor", nil)
	}, "RegisterNative should panic on nil constructor")
}

func TestNativeKeysSorted(t *testing.T) {
	ResetNativeRegistry()
	t.Cleanup(ResetNativeRegistry)

	ctor := func(basePath string) Tool {
		return &testNativeTool{name: "tool"}
	}
	RegisterNative("zed", ctor)
	RegisterNative("alpha", ctor)
	RegisterNative("mu", ctor)
	RegisterNative("beta", ctor)

	keys := NativeKeys()
	require.Equal(t, []string{"alpha", "beta", "mu", "zed"}, keys, "NativeKeys must return sorted keys")
}

func TestNativeKeysEmpty(t *testing.T) {
	ResetNativeRegistry()
	t.Cleanup(ResetNativeRegistry)

	keys := NativeKeys()
	require.Empty(t, keys, "NativeKeys should return empty slice when no tools registered")
}

func TestNativeConstructorExecutes(t *testing.T) {
	ResetNativeRegistry()
	t.Cleanup(ResetNativeRegistry)

	RegisterNative("greeter", func(basePath string) Tool {
		return &testNativeTool{name: "greeter"}
	})

	ctor, ok := LookupNative("greeter")
	require.True(t, ok)

	tool := ctor("/workspace")
	require.Equal(t, "greeter", tool.Name())
	require.True(t, tool.IsAvailable(context.Background()))

	result, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "greeter", result.Data["name"])
}

func TestNativeResetClearsRegistry(t *testing.T) {
	ResetNativeRegistry()

	ctor := func(basePath string) Tool {
		return &testNativeTool{name: "tmp"}
	}
	RegisterNative("temp_tool", ctor)
	require.NotEmpty(t, NativeKeys())

	ResetNativeRegistry()
	require.Empty(t, NativeKeys())

	_, ok := LookupNative("temp_tool")
	require.False(t, ok, "after reset, LookupNative must return false")
}
