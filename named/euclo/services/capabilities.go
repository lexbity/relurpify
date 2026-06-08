package services

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/execution/agentenv"
	"codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
)

// eucloCapabilities is the full self-declared capability set for euclo.
// It is registered unconditionally at initialization; availability of each
// capability is computed at registration time from required tool presence.
var eucloCapabilities = []string{
	"euclo:cap.test_run",
	"euclo:cap.ast_query",
	"euclo:cap.symbol_trace",
	"euclo:cap.call_graph",
	"euclo:cap.blame_trace",
	"euclo:cap.bisect",
	"euclo:cap.code_review",
	"euclo:cap.diff_summary",
	"euclo:cap.layer_check",
	"euclo:cap.targeted_refactor",
	"euclo:cap.rename_symbol",
	"euclo:cap.api_compat",
	"euclo:cap.boundary_report",
	"euclo:cap.coverage_check",
}

// defaultCapabilityRegistrar implements CapabilityRegistrar using Euclo's relurpic capabilities.
type defaultCapabilityRegistrar struct{}

func (r *defaultCapabilityRegistrar) RegisterAll(env agentenv.AgentContext) error {
	if env.Registry == nil {
		return fmt.Errorf("capability registry is nil")
	}
	return relurpicabilities.RegisterAll(env, eucloCapabilities)
}
