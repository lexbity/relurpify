package convert

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	cfg "codeburg.org/lexbit/relurpify/userconfig/config"
	cfgmodel "codeburg.org/lexbit/relurpify/userconfig/config/model"
)

func TestConvertProfileConfigNilReturnsNil(t *testing.T) {
	assert.Nil(t, ConvertProfileConfig(nil))
}

func TestConvertProfileConfigMapsToolCallingIntent(t *testing.T) {
	cases := []struct {
		name       string
		intent     string
		wantNative bool
	}{
		{"native enables native api", "native", true},
		{"prompt_based disables native api", "prompt_based", false},
		{"auto disables native api", "auto", false},
		{"whitespace and case are normalized", "  NATIVE  ", true},
		{"unknown intent leaves native api off", "psychic", false},
		{"empty intent leaves native api off", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := &cfgmodel.ModelProfileConfig{}
			config.ToolCalling.Intent = tc.intent

			profile := ConvertProfileConfig(config)
			require.NotNil(t, profile)
			assert.Equal(t, tc.wantNative, profile.ToolCalling.NativeAPI)
		})
	}
}

func TestConvertProfileConfigMapsQuirksAndContext(t *testing.T) {
	config := &cfgmodel.ModelProfileConfig{}
	config.Pattern = "  gpt-*  "
	config.SourcePath = "/cfg/model/profiles/gpt.llm.yaml"
	config.ToolCalling.Intent = "prompt_based"
	config.ToolCalling.DoubleEncodeArgs = true
	config.ToolCalling.MaxConcurrentTools = 7
	config.Context.MaxTokens = 32100
	config.Repair.Strategy = "  LLM  "

	profile := ConvertProfileConfig(config)

	require.NotNil(t, profile)
	assert.Equal(t, "gpt-*", profile.Pattern, "pattern is trimmed by ModelProfile.Normalize")
	assert.Equal(t, "/cfg/model/profiles/gpt.llm.yaml", profile.SourcePath)
	assert.False(t, profile.ToolCalling.NativeAPI)
	assert.True(t, profile.ToolCalling.DoubleEncodedArgs)
	assert.Equal(t, 7, profile.ToolCalling.MaxToolsPerCall)
	assert.Equal(t, 32100, profile.ContextSize)
	assert.Equal(t, "llm", profile.Repair.Strategy, "repair strategy is lowercased and trimmed")
}

func TestConvertProfileConfigZeroContextLeavesContextSizeZero(t *testing.T) {
	config := &cfgmodel.ModelProfileConfig{}
	config.Context.MaxTokens = 0

	profile := ConvertProfileConfig(config)
	require.NotNil(t, profile)
	assert.Equal(t, 0, profile.ContextSize, "a non-positive max_tokens must not set the context size")
}

func TestConvertProfileConfigNormalizesDefaults(t *testing.T) {
	config := &cfgmodel.ModelProfileConfig{}

	profile := ConvertProfileConfig(config)

	require.NotNil(t, profile)
	assert.Equal(t, "heuristic-only", profile.Repair.Strategy, "an empty repair strategy is normalized by ModelProfile.Normalize")
}

func TestConvertAgentSpecNilReturnsNil(t *testing.T) {
	assert.Nil(t, ConvertAgentSpec(nil))
}

func TestConvertAgentSpecMapsAllSections(t *testing.T) {
	native := true
	in := &cfg.AgentSpec{
		Implementation: "react",
		Version:        "2.1",
		Prompt:         "be helpful",
		Model: cfg.AgentModelConfig{
			Provider:    "openai",
			Name:        "gpt-x",
			Temperature: 0.4,
			MaxTokens:   4096,
		},
		ToolExecutionPolicy: map[string]cfg.ToolPolicy{
			"bash":  {Execute: cfg.AgentPermissionAllow},
			"fetch": {Execute: cfg.AgentPermissionDeny},
		},
		ProviderPolicies: map[string]cfg.ProviderPolicy{
			"mcp-github": {
				Activate:               cfg.AgentPermissionAsk,
				DefaultTrust:           "  WORKSPACE_TRUSTED  ",
				AllowCredentialSharing: true,
			},
		},
		Bash: cfg.AgentBashPermissions{
			AllowPatterns: []string{"go test *"},
			DenyPatterns:  []string{"rm -rf *"},
			Default:       cfg.AgentPermissionAsk,
		},
		NativeToolCalling: &native,
		Logging: &cfg.AgentLoggingSpec{
			LLM: &[]bool{false}[0],
		},
	}

	out := ConvertAgentSpec(in)

	require.NotNil(t, out)
	assert.Equal(t, "react", out.Implementation)
	assert.Equal(t, "2.1", out.Version)
	assert.Equal(t, "be helpful", out.Prompt)
	assert.Equal(t, agentspec.AgentModelConfig{
		Provider:    "openai",
		Name:        "gpt-x",
		Temperature: 0.4,
		MaxTokens:   4096,
	}, out.Model)
	assert.Equal(t, map[string]agentspec.ToolPolicy{
		"bash":  {Execute: agentspec.AgentPermissionAllow},
		"fetch": {Execute: agentspec.AgentPermissionDeny},
	}, out.ToolExecutionPolicy)
	assert.Equal(t, map[string]agentspec.ProviderPolicy{
		// Default trust runs through the classification normalizer:
		// lowercase, trimmed, underscores become dashes.
		"mcp-github": {
			Activate:               agentspec.AgentPermissionAsk,
			DefaultTrust:           agentspec.TrustClassWorkspaceTrusted,
			AllowCredentialSharing: true,
		},
	}, out.ProviderPolicies)
	assert.Equal(t, []string{"go test *"}, out.Bash.AllowPatterns)
	assert.Equal(t, []string{"rm -rf *"}, out.Bash.DenyPatterns)
	assert.Equal(t, agentspec.AgentPermissionLevel(cfg.AgentPermissionAsk), out.Bash.Default)
	require.NotNil(t, out.NativeToolCalling)
	assert.True(t, *out.NativeToolCalling)
	require.NotNil(t, out.Logging)
	require.NotNil(t, out.Logging.LLM)
	assert.False(t, *out.Logging.LLM)
	assert.Nil(t, out.Logging.Agent)
}

func TestConvertAgentSpecBashPatternsAreCopiedNotAliased(t *testing.T) {
	in := &cfg.AgentSpec{
		Bash: cfg.AgentBashPermissions{
			AllowPatterns: []string{"go *"},
			DenyPatterns:  []string{"sudo *"},
		},
	}

	out := ConvertAgentSpec(in)
	require.NotNil(t, out)

	in.Bash.AllowPatterns[0] = "mutated"
	in.Bash.DenyPatterns[0] = "mutated"

	assert.Equal(t, []string{"go *"}, out.Bash.AllowPatterns, "allow patterns must be defensively copied")
	assert.Equal(t, []string{"sudo *"}, out.Bash.DenyPatterns, "deny patterns must be defensively copied")
}

func TestConvertAgentSpecLoggingVariants(t *testing.T) {
	t.Run("nil logging stays nil", func(t *testing.T) {
		out := ConvertAgentSpec(&cfg.AgentSpec{})
		require.NotNil(t, out)
		assert.Nil(t, out.Logging)
	})

	t.Run("agent-only logging", func(t *testing.T) {
		flag := true
		out := ConvertAgentSpec(&cfg.AgentSpec{Logging: &cfg.AgentLoggingSpec{Agent: &flag}})
		require.NotNil(t, out.Logging)
		assert.Nil(t, out.Logging.LLM)
		require.NotNil(t, out.Logging.Agent)
		assert.True(t, *out.Logging.Agent)
	})
}

func TestConvertAgentSpecNativeToolCallingVariants(t *testing.T) {
	t.Run("nil pointer stays nil", func(t *testing.T) {
		out := ConvertAgentSpec(&cfg.AgentSpec{})
		require.NotNil(t, out)
		assert.Nil(t, out.NativeToolCalling)
	})

	t.Run("false pointer is carried over", func(t *testing.T) {
		disabled := false
		out := ConvertAgentSpec(&cfg.AgentSpec{NativeToolCalling: &disabled})
		require.NotNil(t, out.NativeToolCalling)
		assert.False(t, *out.NativeToolCalling)
	})
}

func TestConvertAgentSpecPopulatesDefaultContextPolicy(t *testing.T) {
	// cfg.AgentSpec carries no context section, so the conversion must fill
	// in a safe built-in default rather than leaving the policy unset.
	out := ConvertAgentSpec(&cfg.AgentSpec{})

	require.NotNil(t, out)
	require.NotNil(t, out.Context)
	assert.Equal(t, 100000, out.Context.MaxTokens)
	assert.Equal(t, "balanced", out.Context.CompilationMode)
	assert.Equal(t, agentspec.TrustClassBuiltinTrusted, out.Context.DefaultTrustClass)
}

func TestConvertAgentSpecEmptyMapsAreInitialized(t *testing.T) {
	out := ConvertAgentSpec(&cfg.AgentSpec{})

	require.NotNil(t, out)
	assert.NotNil(t, out.ToolExecutionPolicy)
	assert.NotNil(t, out.ProviderPolicies)
	assert.Empty(t, out.ToolExecutionPolicy)
	assert.Empty(t, out.ProviderPolicies)
}
