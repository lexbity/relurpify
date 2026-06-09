package envcomposition

import (
	"fmt"

	regpkg "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
)

// eucloCapabilityIDs is the full set of Euclo relurpic capability IDs.
var eucloCapabilityIDs = []string{
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

// BuildRelurpicRegistration registers all Euclo relurpic capabilities with the
// given capability registry.
func BuildRelurpicRegistration(reg *regpkg.CapabilityRegistry) error {
	if reg == nil {
		return fmt.Errorf("capability registry required")
	}
	return relurpicabilities.RegisterAll(relurpicabilities.RegistrationDeps{
		Registry: reg,
		Declared: eucloCapabilityIDs,
	})
}
