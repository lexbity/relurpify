package security

import "codeburg.org/lexbit/relurpify/framework/sandbox"

// SubprocessEnvAllowlist returns the normalized list of environment keys that
// may be inherited by subprocesses.
//
// The sandbox policy already validates key uniqueness and emptiness. This
// helper exists so wiring code can pass the canonical allowlist to subprocess
// runners without reaching back into the policy object.
func SubprocessEnvAllowlist(policy *sandbox.SandboxPolicy) []string {
	if policy == nil || len(policy.AllowedEnvKeys) == 0 {
		return nil
	}
	return append([]string(nil), policy.AllowedEnvKeys...)
}
