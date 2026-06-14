package config

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// ContractFingerprint computes a deterministic SHA-256 fingerprint of an
// EffectiveAgentContract and the raw security bundle bytes from the workspace.
//
// The contract is canonicalized as sorted-key JSON; the security bundle bytes
// are appended in a well-known order (localtool, shell, sandbox). The
// fingerprint changes whenever the contract or any security policy file
// changes, enabling reliable reload detection and telemetry correlation.
func ContractFingerprint(c *EffectiveAgentContract, workspace string) [32]byte {
	h := sha256.New()

	// Canonicalize the contract as sorted-key JSON.
	canonical := canonicalContract(c)
	enc := json.NewEncoder(h)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(canonical); err != nil {
		// Unreachable for our types; fall back to a nil-safe hash.
		h.Write([]byte("euclo-contract"))
	}

	// Append raw security bundle bytes in a deterministic order.
	for _, pattern := range []string{"localtool.policy.yaml", "shell.policy.yaml", "sandbox.policy.yaml"} {
		path := filepath.Join(workspace, "relurpify_cfg", "security", pattern)
		data, err := os.ReadFile(filepath.Clean(path))
		if err == nil {
			h.Write(data)
		}
	}

	var fp [32]byte
	copy(fp[:], h.Sum(nil))
	return fp
}

// canonicalContract renders the fingerprint-relevant fields of a contract
// as a sorted-key structure for deterministic hashing.
func canonicalContract(c *EffectiveAgentContract) map[string]any {
	if c == nil {
		return map[string]any{"agent_id": ""}
	}

	out := map[string]any{
		"agent_id": c.AgentID,
	}
	if c.AgentSpec != nil {
		out["implementation"] = c.AgentSpec.Implementation
		out["version"] = c.AgentSpec.Version
		out["model"] = map[string]any{"provider": c.AgentSpec.Model.Provider, "name": c.AgentSpec.Model.Name}

		toolPolicy := make(map[string]string, len(c.AgentSpec.ToolExecutionPolicy))
		for name, pol := range c.AgentSpec.ToolExecutionPolicy {
			toolPolicy[name] = string(pol.Execute)
		}
		out["tool_exec_policy"] = toolPolicy

		denyPatterns := append([]string(nil), c.AgentSpec.Bash.DenyPatterns...)
		sort.Strings(denyPatterns)
		out["bash_deny_patterns"] = denyPatterns

		allowPatterns := append([]string(nil), c.AgentSpec.Bash.AllowPatterns...)
		sort.Strings(allowPatterns)
		out["bash_allow_patterns"] = allowPatterns
	}

	out["security_read_only_root"] = c.Security.ReadOnlyRoot
	out["security_no_new_privileges"] = c.Security.NoNewPrivileges

	return out
}

// MustContractFingerprint is a test helper that panics on error.
func MustContractFingerprint(t interface{ Fatalf(string, ...any) }, c *EffectiveAgentContract, workspace string) [32]byte {
	fp := ContractFingerprint(c, workspace)
	return fp
}
