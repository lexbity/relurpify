package services

import (
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/model"
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
	"euclo:cap.targeted_refactor",
	"euclo:cap.rename_symbol",
	"euclo:cap.api_compat",
	"euclo:cap.coverage_check",
}

// CapabilityDeps carries the session-level dependencies required to construct
// live capability handlers. All fields are optional; handlers degrade
// gracefully to "service not available" when their deps are nil.
type CapabilityDeps struct {
	IndexManager  *ast.IndexManager
	Workspace     string
	CommandRunner relurpicabilities.CommandRuntime
	CommandPolicy relurpicabilities.CommandPolicy
	Model         model.LanguageModel
}

// defaultCapabilityRegistrar implements CapabilityRegistrar using Euclo's relurpic capabilities.
type defaultCapabilityRegistrar struct {
	deps CapabilityDeps
}

func (r *defaultCapabilityRegistrar) RegisterAll(reg *registry.CapabilityRegistry) error {
	return relurpicabilities.RegisterAll(relurpicabilities.RegistrationDeps{
		Registry:      reg,
		Declared:      eucloCapabilities,
		IndexManager:  r.deps.IndexManager,
		Workspace:     r.deps.Workspace,
		CommandRunner: r.deps.CommandRunner,
		CommandPolicy: r.deps.CommandPolicy,
		Model:         r.deps.Model,
	})
}
