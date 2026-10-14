package convert

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfgmodel "codeburg.org/lexbit/relurpify/userconfig/config/model"
)

// FuzzConvertProfileConfig drives the profile conversion with arbitrary
// untrusted pattern/intent/strategy strings and numeric budgets. Invariants:
// the conversion is total for non-nil configs, the repair strategy is always
// normalized to a non-empty value, native tool-calling is enabled exactly when
// the trimmed/lowercased intent is "native", and the context size tracks a
// positive max_tokens only.
func FuzzConvertProfileConfig(f *testing.F) {
	f.Add("gpt-*", "native", "", 128, 4)
	f.Add("", "prompt_based", "LLM", 0, -1)
	f.Add("*", "  AUTO  ", " heuristic-only ", 100000, 32)
	f.Add("\x00\xff", "weird", "\t", -5, 99)

	f.Fuzz(func(t *testing.T, pattern, intent, strategy string, maxTokens, maxTools int) {
		config := &cfgmodel.ModelProfileConfig{}
		config.Pattern = pattern
		config.ToolCalling.Intent = intent
		config.ToolCalling.MaxConcurrentTools = maxTools
		config.ToolCalling.DoubleEncodeArgs = true
		config.Context.MaxTokens = maxTokens
		config.Repair.Strategy = strategy

		profile := ConvertProfileConfig(config)
		require.NotNil(t, profile)

		assert.Equal(t, pattern, profile.Pattern)
		assert.Equal(t, maxTools, profile.ToolCalling.MaxToolsPerCall)
		assert.True(t, profile.ToolCalling.DoubleEncodedArgs)
		assert.Equal(t, strings.ToLower(strings.TrimSpace(intent)) == "native", profile.ToolCalling.NativeAPI)
		if maxTokens > 0 {
			assert.Equal(t, maxTokens, profile.ContextSize)
		} else {
			assert.Equal(t, 0, profile.ContextSize)
		}
		assert.NotEmpty(t, profile.Repair.Strategy,
			"repair strategy must be normalized to a non-empty value")
	})
}
