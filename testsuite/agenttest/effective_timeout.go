package agenttest

import "time"

// DefaultCaseTimeout is the effective per-case execution bound when neither
// the case, the suite, nor the CLI names one (D9.2): 15 minutes — 10x the
// slowest observed smoke case. Unbounded execution requires the explicit
// literal-zero opt-out, never an absence of configuration.
const DefaultCaseTimeout = 15 * time.Minute

// EffectiveTimeout resolves the per-case execution timeout (D9.2):
//
//	case timeout (suite YAML) > suite execution.timeout > CLI --timeout > DefaultCaseTimeout
//
// The FIRST SET value wins, and a literal 0 in that value DISABLES the bound
// (zero means "explicitly unbounded", never "unset" — the empty strings of
// the YAML surfaces and an unset CLI flag map to unset here). Case and suite
// timeouts are duration-validated at suite load, so only set-ness reaches
// this function.
func EffectiveTimeout(caseTimeout, suiteTimeout string, cli time.Duration, cliSet bool) time.Duration {
	if v, ok := parseSetTimeout(caseTimeout); ok {
		return v
	}
	if v, ok := parseSetTimeout(suiteTimeout); ok {
		return v
	}
	if cliSet {
		return cli
	}
	return DefaultCaseTimeout
}

// parseSetTimeout maps an optional timeout string to (value, set). An empty
// string is unset; "0" is set-and-zero (the disable opt-out).
func parseSetTimeout(raw string) (time.Duration, bool) {
	if raw == "" {
		return 0, false
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		// Validation rejects malformed values before this runs.
		return 0, false
	}
	if d == 0 {
		return 0, true
	}
	return d, true
}
