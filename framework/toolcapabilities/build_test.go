package toolcapabilities

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

type toolWithKeys struct {
	contracts.Tool
	keys []string
}

func (t *toolWithKeys) ParamKeys() []string { return t.keys }

type toolWithoutKeys struct {
	contracts.Tool
}

func TestAssertParamKeysPassesWhenConsumedKeysExist(t *testing.T) {
	impl := &toolWithKeys{keys: []string{"pattern", "directory"}}
	params := []contracts.ToolParameter{
		{Name: "pattern", Type: "string"},
		{Name: "directory", Type: "string"},
	}
	err := AssertParamKeys(impl, "search_grep", params)
	require.NoError(t, err)
}

func TestAssertParamKeysFailsOnMissingKey(t *testing.T) {
	impl := &toolWithKeys{keys: []string{"pattern", "missing_key"}}
	params := []contracts.ToolParameter{
		{Name: "pattern", Type: "string"},
	}
	err := AssertParamKeys(impl, "bad_tool", params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad_tool")
	require.Contains(t, err.Error(), "missing_key")
}

func TestAssertParamKeysSkipsWhenNoParamKeysProvider(t *testing.T) {
	impl := &toolWithoutKeys{}
	err := AssertParamKeys(impl, "simple_tool", nil)
	require.NoError(t, err)
}

func TestAssertParamKeysSkipsWhenNoConsumedKeys(t *testing.T) {
	impl := &toolWithKeys{keys: nil}
	err := AssertParamKeys(impl, "no_params", nil)
	require.NoError(t, err)
}

func TestAssertParamKeysNormalizesNames(t *testing.T) {
	impl := &toolWithKeys{keys: []string{"working_directory"}}
	params := []contracts.ToolParameter{
		{Name: "working_directory", Type: "string"},
	}
	err := AssertParamKeys(impl, "go_test", params)
	require.NoError(t, err)
}

func TestAssertParamKeysOnConstructorPasses(t *testing.T) {
	manifest := contracts.ToolManifest{
		Name: "search_grep",
		Parameters: []contracts.ToolParameter{
			{Name: "pattern", Type: "string", Required: true},
		},
	}
	ctor := func(basePath string) contracts.Tool {
		return &toolWithKeys{keys: []string{"pattern"}}
	}
	err := AssertParamKeysOnConstructor("search_grep", ctor, manifest)
	require.NoError(t, err)
}

func TestAssertParamKeysOnConstructorFails(t *testing.T) {
	manifest := contracts.ToolManifest{
		Name: "bad_tool",
		Parameters: []contracts.ToolParameter{
			{Name: "declared", Type: "string"},
		},
	}
	ctor := func(basePath string) contracts.Tool {
		return &toolWithKeys{keys: []string{"undeclared"}}
	}
	err := AssertParamKeysOnConstructor("bad_tool", ctor, manifest)
	require.Error(t, err)
	require.Contains(t, err.Error(), "undeclared")
}

func TestAssertParamKeysOnConstructorNilCtor(t *testing.T) {
	err := AssertParamKeysOnConstructor("nil", nil, contracts.ToolManifest{Name: "nil"})
	require.NoError(t, err)
}

// --- Build tests ---

type nopRunner struct{}

func (n *nopRunner) Run(_ context.Context, _ contracts.CommandRequest) (*contracts.CommandResult, error) {
	return &contracts.CommandResult{}, nil
}

func TestBuildSubprocessTool(t *testing.T) {
	tools := Build("/ws", &nopRunner{}, []*contracts.ToolManifest{
		{
			Name:        "cli_echo",
			Family:      "text",
			Description: "echo",
			Execution: contracts.ToolManifestExecution{
				Backend: contracts.ToolBackendSubprocess,
				Command: &contracts.ToolManifestCommand{Base: []string{"echo"}},
			},
			Capability: contracts.ToolManifestCapability{
				TrustClass:  "builtin_trusted",
				RiskClass:   []string{"execute"},
				EffectClass: []string{"filesystem_read"},
			},
		},
	})
	require.Len(t, tools, 1)
	require.Equal(t, "cli_echo", tools[0].Name())
}

func TestBuildSubprocessToolMissingDescription(t *testing.T) {
	tools := Build("/ws", &nopRunner{}, []*contracts.ToolManifest{
		{
			Name:   "no_desc",
			Family: "text",
			Execution: contracts.ToolManifestExecution{
				Backend: contracts.ToolBackendSubprocess,
				Command: &contracts.ToolManifestCommand{Base: []string{"tool"}},
			},
			Capability: contracts.ToolManifestCapability{
				TrustClass:  "builtin_trusted",
				RiskClass:   []string{"execute"},
				EffectClass: []string{"filesystem_read"},
			},
		},
	})
	require.Empty(t, tools, "tool with empty description must be excluded")
}

func TestBuildCompositeTool(t *testing.T) {
	tools := Build("/ws", nil, []*contracts.ToolManifest{
		{
			Name:        "pipe_tool",
			Family:      "shell",
			Description: "pipeline",
			Execution: contracts.ToolManifestExecution{
				Backend: contracts.ToolBackendComposite,
			},
			Capability: contracts.ToolManifestCapability{
				TrustClass:  "builtin_trusted",
				RiskClass:   []string{"execute"},
				EffectClass: []string{"filesystem_read"},
			},
			Composition: &contracts.ToolManifestComposition{
				Steps: []contracts.ToolManifestCompositionStep{
					{Tool: "cli_fd", Alias: "files"},
				},
			},
		},
	})
	require.Len(t, tools, 1)
	require.Equal(t, "pipe_tool", tools[0].Name())
	require.Equal(t, "shell", tools[0].Category(), "composite tool category comes from manifest family")
}

func TestBuildNonStrictModeGoNativeMissingImplSkipped(t *testing.T) {
	// Without StrictMode, a missing implementation is logged and skipped (not hard-fail)
	tools := Build("/ws", nil, []*contracts.ToolManifest{
		{
			Name:        "missing_impl",
			Family:      "search",
			Description: "missing",
			Execution: contracts.ToolManifestExecution{
				Backend:        contracts.ToolBackendGoNative,
				Implementation: "does_not_exist",
			},
			Capability: contracts.ToolManifestCapability{
				TrustClass:  "builtin_trusted",
				RiskClass:   []string{"read_only"},
				EffectClass: []string{"filesystem_read"},
			},
		},
	})
	require.Empty(t, tools, "non-strict mode must skip tools with unregistered implementations")
}

func TestBuildStrictModeGoNativeMissingImpl(t *testing.T) {
	tools := Build("/ws", nil, []*contracts.ToolManifest{
		{
			Name:        "missing_impl",
			Family:      "search",
			Description: "missing",
			Execution: contracts.ToolManifestExecution{
				Backend:        contracts.ToolBackendGoNative,
				Implementation: "does_not_exist",
			},
			Capability: contracts.ToolManifestCapability{
				TrustClass:  "builtin_trusted",
				RiskClass:   []string{"read_only"},
				EffectClass: []string{"filesystem_read"},
			},
		},
	}, StrictMode())
	require.Empty(t, tools, "strict mode must exclude tools with unregistered implementations")
}

func TestBuildGoNativeToolFromRegistry(t *testing.T) {
	// Register a test native tool for the duration of this test
	contracts.ResetNativeRegistry()
	t.Cleanup(contracts.ResetNativeRegistry)

	contracts.RegisterNative("test_greeter", func(basePath string) contracts.Tool {
		return &testTool{name: "test_greeter"}
	})

	tools := Build("/ws", nil, []*contracts.ToolManifest{
		{
			Name:        "test_greeter",
			Family:      "test",
			Description: "greeter",
			Execution: contracts.ToolManifestExecution{
				Backend:        contracts.ToolBackendGoNative,
				Implementation: "test_greeter",
			},
			Capability: contracts.ToolManifestCapability{
				TrustClass:  "builtin_trusted",
				RiskClass:   []string{"read_only"},
				EffectClass: []string{"filesystem_read"},
			},
		},
	})
	require.Len(t, tools, 1)
	require.Equal(t, "test_greeter", tools[0].Name())
}

func TestBuildExcludesNilManifests(t *testing.T) {
	tools := Build("/ws", &nopRunner{}, []*contracts.ToolManifest{nil})
	require.Empty(t, tools)
}

func TestBuildExcludesEmptyName(t *testing.T) {
	tools := Build("/ws", &nopRunner{}, []*contracts.ToolManifest{
		{
			Name:   "",
			Family: "text",
			Execution: contracts.ToolManifestExecution{
				Backend: contracts.ToolBackendSubprocess,
				Command: &contracts.ToolManifestCommand{Base: []string{"echo"}},
			},
			Capability: contracts.ToolManifestCapability{
				TrustClass:  "builtin_trusted",
				RiskClass:   []string{"execute"},
				EffectClass: []string{"filesystem_read"},
			},
		},
	})
	require.Empty(t, tools)
}

// --- Capability class provider tests ---

func TestWrapWithCapabilityProvidesManifestTrustClass(t *testing.T) {
	tool := wrapWithCapability(
		&testTool{name: "trusted_tool"},
		contracts.ToolManifest{
			Capability: contracts.ToolManifestCapability{
				TrustClass: "untrusted",
			},
		},
	)
	provider, ok := tool.(interface{ TrustClass() agentspec.TrustClass })
	require.True(t, ok, "wrapped tool must implement TrustClass provider")
	require.Equal(t, agentspec.TrustClass("untrusted"), provider.TrustClass())
}

func TestWrapWithCapabilityProvidesManifestRiskClasses(t *testing.T) {
	tool := wrapWithCapability(
		&testTool{name: "risk_tool"},
		contracts.ToolManifest{
			Capability: contracts.ToolManifestCapability{
				RiskClass: []string{"execute", "network"},
			},
		},
	)
	provider, ok := tool.(interface{ RiskClasses() []agentspec.RiskClass })
	require.True(t, ok)
	require.Equal(t, []agentspec.RiskClass{"execute", "network"}, provider.RiskClasses())
}

func TestWrapWithCapabilityProvidesManifestEffectClasses(t *testing.T) {
	tool := wrapWithCapability(
		&testTool{name: "effect_tool"},
		contracts.ToolManifest{
			Capability: contracts.ToolManifestCapability{
				EffectClass: []string{"filesystem_read", "process_spawn"},
			},
		},
	)
	provider, ok := tool.(interface{ EffectClasses() []agentspec.EffectClass })
	require.True(t, ok)
	require.Equal(t, []agentspec.EffectClass{"filesystem_read", "process_spawn"}, provider.EffectClasses())
}

func TestWrapWithCapabilityReturnsOriginalWhenNoCapability(t *testing.T) {
	original := &testTool{name: "plain"}
	wrapped := wrapWithCapability(original, contracts.ToolManifest{})
	require.Same(t, original, wrapped, "must return original tool when no capability data")
}

func TestWrapWithCapabilityDelegatesToUnderlyingTool(t *testing.T) {
	tool := wrapWithCapability(
		&testTool{name: "delegate"},
		contracts.ToolManifest{
			Capability: contracts.ToolManifestCapability{
				TrustClass: "untrusted",
			},
		},
	)
	require.Equal(t, "delegate", tool.Name())
	require.Equal(t, "test: delegate", tool.Description())
	require.Equal(t, "test", tool.Category())
}

func TestBuildToolGetsCapabilityClassProvider(t *testing.T) {
	contracts.ResetNativeRegistry()
	t.Cleanup(contracts.ResetNativeRegistry)

	contracts.RegisterNative("cap_test", func(basePath string) contracts.Tool {
		return &testTool{name: "cap_test"}
	})

	tools := Build("/ws", nil, []*contracts.ToolManifest{
		{
			Name:        "cap_test",
			Family:      "test",
			Description: "capability test",
			Execution: contracts.ToolManifestExecution{
				Backend:        contracts.ToolBackendGoNative,
				Implementation: "cap_test",
			},
			Capability: contracts.ToolManifestCapability{
				TrustClass:  "untrusted",
				RiskClass:   []string{"execute"},
				EffectClass: []string{"process_spawn"},
			},
		},
	})
	require.Len(t, tools, 1)

	trustProv, ok := tools[0].(interface{ TrustClass() agentspec.TrustClass })
	require.True(t, ok, "Build must wrap tools with TrustClass provider")
	require.Equal(t, agentspec.TrustClass("untrusted"), trustProv.TrustClass())

	riskProv, ok := tools[0].(interface{ RiskClasses() []agentspec.RiskClass })
	require.True(t, ok)
	require.Equal(t, []agentspec.RiskClass{"execute"}, riskProv.RiskClasses())

	effectProv, ok := tools[0].(interface{ EffectClasses() []agentspec.EffectClass })
	require.True(t, ok)
	require.Equal(t, []agentspec.EffectClass{"process_spawn"}, effectProv.EffectClasses())
}

func TestBuildToolWithoutCapabilityNoProvider(t *testing.T) {
	tools := Build("/ws", &nopRunner{}, []*contracts.ToolManifest{
		{
			Name:        "no_cap",
			Family:      "text",
			Description: "no capability",
			Execution: contracts.ToolManifestExecution{
				Backend: contracts.ToolBackendSubprocess,
				Command: &contracts.ToolManifestCommand{Base: []string{"echo"}},
			},
			Capability: contracts.ToolManifestCapability{},
		},
	})
	require.Len(t, tools, 1)
	// Empty capability means no wrapper — original tool returned
	_, ok := tools[0].(interface{ TrustClass() agentspec.TrustClass })
	require.False(t, ok, "subprocess tool without capability should not have TrustClass provider")
}

func TestUnlistedManifestExcluded(t *testing.T) {
	// Tools built from manifests not provided to Build must not appear
	tools := Build("/ws", &nopRunner{}, []*contracts.ToolManifest{
		{
			Name:        "allowed",
			Family:      "text",
			Description: "allowed tool",
			Execution: contracts.ToolManifestExecution{
				Backend: contracts.ToolBackendSubprocess,
				Command: &contracts.ToolManifestCommand{Base: []string{"echo"}},
			},
			Capability: contracts.ToolManifestCapability{
				TrustClass:  "builtin_trusted",
				RiskClass:   []string{"execute"},
				EffectClass: []string{"filesystem_read"},
			},
		},
	})
	require.Len(t, tools, 1)
	require.Equal(t, "allowed", tools[0].Name())
	// A tool not in the manifest list won't be built
}

// testTool is a minimal Tool implementation for Build tests.
type testTool struct {
	contracts.Tool
	name string
}

func (t *testTool) Name() string        { return t.name }
func (t *testTool) Description() string { return "test: " + t.name }
func (t *testTool) Category() string    { return "test" }


