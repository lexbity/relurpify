package runtime

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/safety"
)

func safetyDesc(id, providerID, sessionID string) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:   id,
		Kind: agentspec.CapabilityKindTool,
		Source: descriptor.CapabilitySource{
			ProviderID: providerID,
			SessionID:  sessionID,
		},
	}
}

func TestRuntimeSafetyControllerNilReceiverIsTotal(t *testing.T) {
	var controller *RuntimeSafetyController

	controller.Configure(&safety.RuntimeSafetySpec{MaxCallsPerCapability: 1})
	controller.Configure(nil)
	assert.Nil(t, controller.SnapshotSpec())
	assert.Empty(t, controller.RevocationSnapshot())

	controller.RevokeCapability("cap", "reason")
	controller.RevokeProvider("prov", "reason")
	controller.RevokeSession("sess", "reason")
	controller.ReinstateCapability("cap")
	controller.ReinstateProvider("prov")
	controller.ReinstateSession("sess")

	assert.NoError(t, controller.CheckBeforeExecution(safetyDesc("cap", "prov", "sess")))
	assert.NoError(t, controller.RecordResult(safetyDesc("cap", "prov", "sess"), &ports.ToolResult{}))
	assert.NoError(t, controller.RecordSessionSubprocess("sess", 1))
	assert.NoError(t, controller.RecordSessionNetworkRequest("sess", 1))
	assert.Nil(t, RedactAny(nil))
	assert.Nil(t, RedactMetadataMap(nil))
}

func TestConfigureAndSnapshotSpecCloneToProtectControllerState(t *testing.T) {
	controller := NewRuntimeSafetyController()
	assert.Nil(t, controller.SnapshotSpec(), "unconfigured controller has no spec")

	spec := &safety.RuntimeSafetySpec{MaxCallsPerCapability: 3, MaxBytesPerSession: 100}
	controller.Configure(spec)

	// Mutating the caller's spec after Configure must not affect the controller.
	spec.MaxCallsPerCapability = 99
	snapshotted := controller.SnapshotSpec()
	require.NotNil(t, snapshotted)
	assert.Equal(t, 3, snapshotted.MaxCallsPerCapability)

	// Mutating the returned snapshot must not affect the controller either.
	snapshotted.MaxBytesPerSession = 1
	assert.Equal(t, 100, controller.SnapshotSpec().MaxBytesPerSession)

	// A nil spec clears configuration.
	controller.Configure(nil)
	assert.Nil(t, controller.SnapshotSpec())
}

func TestRevocationLifecycle(t *testing.T) {
	controller := NewRuntimeSafetyController()

	controller.RevokeCapability("cap-1", "security incident")
	controller.RevokeProvider("prov-1", "")
	controller.RevokeSession("sess-1", "session abuse")
	// Empty ids are ignored.
	controller.RevokeCapability("", "nope")
	controller.RevokeProvider("", "nope")
	controller.RevokeSession("", "nope")

	snapshot := controller.RevocationSnapshot()
	assert.Equal(t, map[string]string{"cap-1": "security incident"}, snapshot.Capabilities)
	assert.Equal(t, map[string]string{"prov-1": "revoked by runtime policy"}, snapshot.Providers,
		"an empty reason falls back to the default revocation reason")
	assert.Equal(t, map[string]string{"sess-1": "session abuse"}, snapshot.Sessions)

	// The snapshot is a copy: mutating it must not affect the controller.
	snapshot.Capabilities["cap-2"] = "injected"
	assert.NotContains(t, controller.RevocationSnapshot().Capabilities, "cap-2")

	controller.ReinstateCapability("cap-1")
	controller.ReinstateProvider("prov-1")
	controller.ReinstateSession("sess-1")
	controller.ReinstateCapability("") // no-op on empty id

	cleared := controller.RevocationSnapshot()
	assert.Empty(t, cleared.Capabilities)
	assert.Empty(t, cleared.Providers)
	assert.Empty(t, cleared.Sessions)

	// Reinstate of an unknown id is a no-op.
	controller.ReinstateCapability("ghost")
	assert.Empty(t, controller.RevocationSnapshot().Capabilities)
}

func TestCheckBeforeExecutionRejectsRevokedTargets(t *testing.T) {
	controller := NewRuntimeSafetyController()
	controller.RevokeCapability("cap-1", "compromised")
	controller.RevokeProvider("prov-1", "provider offline")
	controller.RevokeSession("sess-1", "session expired")

	err := controller.CheckBeforeExecution(safetyDesc("cap-1", "prov-1", "sess-1"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "capability cap-1 revoked: compromised",
		"capability revocation is checked first")

	err = controller.CheckBeforeExecution(safetyDesc("cap-2", "prov-1", "sess-1"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider prov-1 revoked: provider offline")

	err = controller.CheckBeforeExecution(safetyDesc("cap-2", "prov-2", "sess-1"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session sess-1 revoked: session expired")

	// Unrevoked combinations pass.
	assert.NoError(t, controller.CheckBeforeExecution(safetyDesc("cap-2", "prov-2", "")))
}

func TestCheckBeforeExecutionEnforcesCallBudgets(t *testing.T) {
	controller := NewRuntimeSafetyController()
	controller.Configure(&safety.RuntimeSafetySpec{
		MaxCallsPerCapability: 2,
		MaxCallsPerProvider:   3,
	})

	// Same capability, same provider as the revoked-free path.
	assert.NoError(t, controller.CheckBeforeExecution(safetyDesc("cap-1", "prov-1", "")))
	assert.NoError(t, controller.CheckBeforeExecution(safetyDesc("cap-1", "prov-1", "")))
	err := controller.CheckBeforeExecution(safetyDesc("cap-1", "prov-1", ""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "capability cap-1 blocked: call budget exceeded")

	// A different capability on the same provider still passes, consuming the
	// third provider call.
	assert.NoError(t, controller.CheckBeforeExecution(safetyDesc("cap-2", "prov-1", "")))
	err = controller.CheckBeforeExecution(safetyDesc("cap-3", "prov-1", ""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider prov-1 blocked: call budget exceeded")

	// A blocked call must not consume more budget, and another provider is
	// unaffected.
	assert.NoError(t, controller.CheckBeforeExecution(safetyDesc("cap-5", "prov-2", "")))
}

func TestCheckBeforeExecutionWithoutSpecAllowsUnlimitedCalls(t *testing.T) {
	controller := NewRuntimeSafetyController()
	for range 20 {
		assert.NoError(t, controller.CheckBeforeExecution(safetyDesc("cap-1", "prov-1", "sess-1")))
	}
}

func TestCheckBeforeExecutionSkipsAccountingForAnonymousDescriptors(t *testing.T) {
	controller := NewRuntimeSafetyController()
	controller.Configure(&safety.RuntimeSafetySpec{MaxCallsPerCapability: 1, MaxCallsPerProvider: 1})

	// No id and no provider: nothing to check, nothing to account.
	assert.NoError(t, controller.CheckBeforeExecution(descriptor.CapabilityDescriptor{Source: descriptor.CapabilitySource{SessionID: "sess"}}))
	assert.NoError(t, controller.CheckBeforeExecution(descriptor.CapabilityDescriptor{Source: descriptor.CapabilitySource{SessionID: "sess"}}))
}

func TestRecordResultAttributesSessionAndEnforcesBudgets(t *testing.T) {
	result := &ports.ToolResult{Success: true, Data: map[string]any{"output": strings.Repeat("x", 100)}}
	payloadBytes := estimatePayloadBytes(result)
	payloadTokens := estimatePayloadTokens(result)
	require.Greater(t, payloadBytes, 0)
	require.Greater(t, payloadTokens, 0)

	t.Run("session id comes from the descriptor first", func(t *testing.T) {
		controller := NewRuntimeSafetyController()
		controller.Configure(&safety.RuntimeSafetySpec{MaxBytesPerSession: payloadBytes * 2})

		// The descriptor session is set, so the result's data session id must
		// be ignored: all three payloads accumulate on one bucket and the
		// third call trips the budget. If attribution wrongly followed the
		// result data, the first payload would land elsewhere and the third
		// call would still pass.
		desc := safetyDesc("cap-1", "prov-1", "sess-descriptor")
		resultWithSession := &ports.ToolResult{Data: map[string]any{"session_id": "sess-result"}}
		require.NoError(t, controller.RecordResult(desc, resultWithSession))
		require.NoError(t, controller.RecordResult(desc, result))
		err := controller.RecordResult(desc, result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "session sess-descriptor blocked: byte budget exceeded")
	})

	t.Run("session id falls back to result data", func(t *testing.T) {
		controller := NewRuntimeSafetyController()
		// Budget against THIS result's payload size: two records fit, the
		// third must trip the budget.
		resultWithSession := &ports.ToolResult{Data: map[string]any{"session_id": "sess-result"}}
		smallBytes := estimatePayloadBytes(resultWithSession)
		controller.Configure(&safety.RuntimeSafetySpec{MaxBytesPerSession: smallBytes * 2})

		desc := safetyDesc("cap-1", "prov-1", "")
		assert.NoError(t, controller.RecordResult(desc, resultWithSession))
		assert.NoError(t, controller.RecordResult(desc, resultWithSession))
		err := controller.RecordResult(desc, resultWithSession)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "session sess-result blocked: byte budget exceeded")
	})

	t.Run("result without data and without descriptor session is ignored", func(t *testing.T) {
		controller := NewRuntimeSafetyController()
		controller.Configure(&safety.RuntimeSafetySpec{MaxBytesPerSession: 1})
		assert.NoError(t, controller.RecordResult(safetyDesc("cap-1", "prov-1", ""), &ports.ToolResult{Success: true}))
		assert.NoError(t, controller.RecordResult(safetyDesc("cap-1", "prov-1", ""), nil))
	})

	t.Run("non-string session id in data is stringified", func(t *testing.T) {
		controller := NewRuntimeSafetyController()
		controller.Configure(&safety.RuntimeSafetySpec{MaxBytesPerSession: 1})

		desc := safetyDesc("cap-1", "prov-1", "")
		resultWithNumericSession := &ports.ToolResult{Data: map[string]any{"session_id": 42}}
		err := controller.RecordResult(desc, resultWithNumericSession)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "session 42 blocked: byte budget exceeded")
	})

	t.Run("result without session id is ignored", func(t *testing.T) {
		controller := NewRuntimeSafetyController()
		controller.Configure(&safety.RuntimeSafetySpec{MaxBytesPerSession: 1})
		assert.NoError(t, controller.RecordResult(safetyDesc("cap-1", "prov-1", ""), result))
	})

	t.Run("controller without spec accepts everything", func(t *testing.T) {
		controller := NewRuntimeSafetyController()
		assert.NoError(t, controller.RecordResult(safetyDesc("cap-1", "prov-1", "sess-1"), result))
	})

	t.Run("byte budget blocks without partial accumulation", func(t *testing.T) {
		controller := NewRuntimeSafetyController()
		controller.Configure(&safety.RuntimeSafetySpec{MaxBytesPerSession: payloadBytes + 1})

		assert.NoError(t, controller.RecordResult(safetyDesc("cap-1", "prov-1", "sess-1"), result))
		err := controller.RecordResult(safetyDesc("cap-1", "prov-1", "sess-1"), result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "byte budget exceeded")
	})

	t.Run("token budget blocks independently of bytes", func(t *testing.T) {
		controller := NewRuntimeSafetyController()
		controller.Configure(&safety.RuntimeSafetySpec{MaxOutputTokensSession: payloadTokens})

		assert.NoError(t, controller.RecordResult(safetyDesc("cap-1", "prov-1", "sess-1"), result))
		err := controller.RecordResult(safetyDesc("cap-1", "prov-1", "sess-1"), result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "output token budget exceeded")
	})
}

func TestRecordSessionSubprocessBudget(t *testing.T) {
	controller := NewRuntimeSafetyController()

	// Without a spec everything is allowed.
	assert.NoError(t, controller.RecordSessionSubprocess("sess-1", 10))
	// Non-positive counts are ignored even with a spec.
	controller.Configure(&safety.RuntimeSafetySpec{MaxSubprocessesPerSession: 3})
	assert.NoError(t, controller.RecordSessionSubprocess("sess-1", 0))
	assert.NoError(t, controller.RecordSessionSubprocess("sess-1", -2))
	// Empty session ids are ignored.
	assert.NoError(t, controller.RecordSessionSubprocess("", 5))

	assert.NoError(t, controller.RecordSessionSubprocess("sess-1", 2))
	assert.NoError(t, controller.RecordSessionSubprocess("sess-1", 1))
	err := controller.RecordSessionSubprocess("sess-1", 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session sess-1 blocked: subprocess budget exceeded")

	// A nil controller is a no-op.
	var nilController *RuntimeSafetyController
	assert.NoError(t, nilController.RecordSessionSubprocess("sess-1", 1))
}

func TestRecordSessionNetworkRequestBudget(t *testing.T) {
	controller := NewRuntimeSafetyController()

	assert.NoError(t, controller.RecordSessionNetworkRequest("sess-1", 10), "no spec configured yet")
	controller.Configure(&safety.RuntimeSafetySpec{MaxNetworkRequestsSession: 2})
	assert.NoError(t, controller.RecordSessionNetworkRequest("sess-1", 0))
	assert.NoError(t, controller.RecordSessionNetworkRequest("sess-1", -1))
	assert.NoError(t, controller.RecordSessionNetworkRequest("", 9))

	assert.NoError(t, controller.RecordSessionNetworkRequest("sess-1", 2))
	err := controller.RecordSessionNetworkRequest("sess-1", 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network request budget exceeded")

	// Subprocess and network buckets are independent.
	subprocessController := NewRuntimeSafetyController()
	subprocessController.Configure(&safety.RuntimeSafetySpec{
		MaxSubprocessesPerSession: 1,
		MaxNetworkRequestsSession: 1,
	})
	assert.NoError(t, subprocessController.RecordSessionSubprocess("s", 1))
	assert.NoError(t, subprocessController.RecordSessionNetworkRequest("s", 1))
}

func TestSortedKeys(t *testing.T) {
	assert.Empty(t, SortedKeys(nil))
	assert.Empty(t, SortedKeys(map[string]string{}))
	assert.Equal(t, []string{"a", "b", "c"}, SortedKeys(map[string]string{"c": "1", "a": "2", "b": "3"}))
}

func TestEstimatePayloadHelpers(t *testing.T) {
	assert.Equal(t, 0, estimatePayloadBytes())
	assert.Equal(t, 4, estimatePayloadBytes("abcd"))
	assert.Equal(t, 4, estimatePayloadBytes("  ab  ", "cd"), "each value is trimmed before counting")
	assert.Equal(t, 1, estimatePayloadTokens("abcd"))
	assert.Equal(t, 2, estimatePayloadTokens("abcd", "abcd"))
	assert.Equal(t, 0, estimatePayloadTokens("abc"), "sub-token payloads round down to zero")
}

func TestSpecLimitHelpersTolerateNilSpec(t *testing.T) {
	controller := NewRuntimeSafetyController()
	assert.Equal(t, 0, controller.specMaxSubprocesses(nil))
	assert.Equal(t, 0, controller.specMaxNetworkRequests(nil))

	controller.Configure(&safety.RuntimeSafetySpec{MaxSubprocessesPerSession: 4, MaxNetworkRequestsSession: 5})
	assert.Equal(t, 4, controller.specMaxSubprocesses(controller.spec))
	assert.Equal(t, 5, controller.specMaxNetworkRequests(controller.spec))
}

func TestCloneReasonMap(t *testing.T) {
	assert.Nil(t, cloneReasonMap(nil))
	input := map[string]string{"a": "1"}
	clone := cloneReasonMap(input)
	assert.Equal(t, input, clone)
	clone["a"] = "2"
	assert.Equal(t, "1", input["a"], "clone must be independent")
}

func TestRedactMetadataMapRedactsSensitiveKeys(t *testing.T) {
	input := map[string]any{
		"api_key":           "sk-123",
		"APIKey":            "whatever",
		"password":          "hunter2",
		"X-Auth-Token":      "tok",
		"authorization":     "Bearer abc",
		"cookie":            "sid=1",
		"client_credential": "c",
		"apikey":            "k",
		"secret":            "s",
		"username":          "alex",
		"count":             7,
		"nested":            map[string]any{"token": "t", "safe": "v"},
		"string_map":        map[string]string{"api_key": "z", "note": "n"},
		"string_slice":      []string{"plain", "bearer xyz"},
		"mixed_slice":       []any{"a", 2, map[string]any{"password": "p"}, []string{"plain"}},
	}

	redacted := RedactMetadataMap(input)

	assert.Equal(t, "[REDACTED]", redacted["api_key"])
	assert.Equal(t, "[REDACTED]", redacted["APIKey"], "key matching is case-insensitive")
	assert.Equal(t, "[REDACTED]", redacted["password"])
	assert.Equal(t, "[REDACTED]", redacted["X-Auth-Token"])
	assert.Equal(t, "[REDACTED]", redacted["authorization"])
	assert.Equal(t, "[REDACTED]", redacted["cookie"])
	assert.Equal(t, "[REDACTED]", redacted["client_credential"])
	assert.Equal(t, "[REDACTED]", redacted["apikey"])
	assert.Equal(t, "[REDACTED]", redacted["secret"])
	assert.Equal(t, "alex", redacted["username"])
	assert.Equal(t, 7, redacted["count"])
	assert.Equal(t, map[string]any{"token": "[REDACTED]", "safe": "v"}, redacted["nested"])
	assert.Equal(t, map[string]any{"api_key": "[REDACTED]", "note": "n"}, redacted["string_map"])
	assert.Equal(t, []any{"plain", "[REDACTED]"}, redacted["string_slice"])
	assert.Equal(t, []any{"a", 2, map[string]any{"password": "[REDACTED]"}, []any{"plain"}}, redacted["mixed_slice"])

	// The input map must not be mutated.
	assert.Equal(t, "sk-123", input["api_key"])
	assert.Equal(t, "t", input["nested"].(map[string]any)["token"])
}

func TestRedactMetadataMapNilReturnsNil(t *testing.T) {
	assert.Nil(t, RedactMetadataMap(nil))
}

func TestRedactAnyDispatchesByType(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		assert.Nil(t, RedactAny(nil))
	})
	t.Run("map any", func(t *testing.T) {
		out := RedactAny(map[string]any{"token": "t", "keep": "v"})
		assert.Equal(t, map[string]any{"token": "[REDACTED]", "keep": "v"}, out)
	})
	t.Run("map string", func(t *testing.T) {
		out := RedactAny(map[string]string{"api_key": "z", "note": "n"})
		assert.Equal(t, map[string]any{"api_key": "[REDACTED]", "note": "n"}, out)
	})
	t.Run("slice of any", func(t *testing.T) {
		out := RedactAny([]any{"ok", map[string]any{"secret": "s"}, 5})
		assert.Equal(t, []any{"ok", map[string]any{"secret": "[REDACTED]"}, 5}, out)
	})
	t.Run("slice of string", func(t *testing.T) {
		out := RedactAny([]string{"plain", "authorization: Basic abc"})
		assert.Equal(t, []any{"plain", "[REDACTED]"}, out)
	})
	t.Run("plain sensitive string", func(t *testing.T) {
		assert.Equal(t, "[REDACTED]", RedactAny("Bearer abc"))
		assert.Equal(t, "hello", RedactAny("hello"))
	})
	t.Run("non string scalars pass through", func(t *testing.T) {
		assert.Equal(t, 42, RedactAny(42))
		assert.Equal(t, true, RedactAny(true))
	})
}

func TestLooksSensitiveValue(t *testing.T) {
	cases := []struct {
		value     string
		sensitive bool
	}{
		{"", false},
		{"plain text", false},
		{"Bearer abcdef", true},
		{"bearer abcdef", true},
		{"Authorization: Basic xyz", true},
		{"sid=1; session=abc", true},
		{"sk-abc123", true},
		{"pk-live-xyz", true},
		{"ghp_1234567890", true},
		{"github_pat_11AA22BB", true},
		{"ghu_token", true},
		{"gho_token", true},
		{"ghb_token", true},
		{"AKIA123456", true},
		{"eyJhbGciOiJIUzI1NiJ9.payload", true},
		{"export API_KEY=xyz", true},
		{"apikey=abcd", true},
		{"x-api-key: 1", true},
		{"  ", false},
		// Prose only matches when a literal needle ("api_key", "apikey",
		// "api-key", ...) appears; plain words do not trigger redaction.
		{"NOTE about api keys in general docs", false},
	}

	for _, tc := range cases {
		assert.Equal(t, tc.sensitive, looksSensitiveValue(tc.value), "looksSensitiveValue(%q)", tc.value)
	}
}

func TestIsSensitiveKey(t *testing.T) {
	cases := []struct {
		key       string
		sensitive bool
	}{
		{"", false},
		{"username", false},
		{"count", false},
		{"api_key", true},
		{"API_KEY", true},
		{"  token  ", true},
		{"Session-Token", true},
		{"openai-apikey", true},
		{"client_secret", true},
		{"Set-Cookie", true},
		{"Authorization", true},
		{"password_hash", true},
		{"x_credential", true},
		{"authority", true},
	}

	for _, tc := range cases {
		assert.Equal(t, tc.sensitive, isSensitiveKey(tc.key), "isSensitiveKey(%q)", tc.key)
	}
}

func TestRevocationSnapshotFeedsPolicySnapshotVocabulary(t *testing.T) {
	// The revocation snapshot type shared with capability/result round-trips
	// through the runtime controller's snapshots.
	controller := NewRuntimeSafetyController()
	controller.RevokeCapability("cap-1", "r1")
	controller.RevokeProvider("prov-1", "r2")
	controller.RevokeSession("sess-1", "r3")

	snapshot := controller.RevocationSnapshot()
	assert.Equal(t, map[string]string{"cap-1": "r1"}, snapshot.Capabilities)
	assert.Equal(t, map[string]string{"prov-1": "r2"}, snapshot.Providers)
	assert.Equal(t, map[string]string{"sess-1": "r3"}, snapshot.Sessions)
}
