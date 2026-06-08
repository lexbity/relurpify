package relurpicabilities

import (
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/handler"

	"codeburg.org/lexbit/relurpify/execution/agentenv"
)

type relurpicCapabilityBlueprint struct {
	ID            string
	RequiredTools []string
	NewHandler    func(agentenv.AgentContext) handler.InvocableCapabilityHandler
}

var eucloRelurpicCapabilityBlueprints = []relurpicCapabilityBlueprint{
	{ID: "euclo:cap.test_run", RequiredTools: []string{"file_read"}, NewHandler: func(env agentenv.AgentContext) handler.InvocableCapabilityHandler {
		return NewTestRunHandler(env)
	}},
	{ID: "euclo:cap.ast_query", RequiredTools: []string{"file_read"}, NewHandler: func(env agentenv.AgentContext) handler.InvocableCapabilityHandler {
		return NewASTQueryHandler(env)
	}},
	{ID: "euclo:cap.symbol_trace", RequiredTools: []string{"file_read"}, NewHandler: func(env agentenv.AgentContext) handler.InvocableCapabilityHandler {
		return NewSymbolTraceHandler(env)
	}},
	{ID: "euclo:cap.call_graph", RequiredTools: []string{"file_read"}, NewHandler: func(env agentenv.AgentContext) handler.InvocableCapabilityHandler {
		return NewCallGraphHandler(env)
	}},
	{ID: "euclo:cap.blame_trace", RequiredTools: []string{"file_read"}, NewHandler: func(env agentenv.AgentContext) handler.InvocableCapabilityHandler {
		return NewBlameTraceHandler(env)
	}},
	{ID: "euclo:cap.bisect", RequiredTools: []string{"file_read"}, NewHandler: func(env agentenv.AgentContext) handler.InvocableCapabilityHandler {
		return NewBisectHandler(env)
	}},
	{ID: "euclo:cap.code_review", RequiredTools: []string{"file_read"}, NewHandler: func(env agentenv.AgentContext) handler.InvocableCapabilityHandler {
		return NewCodeReviewHandler(env)
	}},
	{ID: "euclo:cap.diff_summary", RequiredTools: []string{"file_read"}, NewHandler: func(env agentenv.AgentContext) handler.InvocableCapabilityHandler {
		return NewDiffSummaryHandler(env)
	}},
	{ID: "euclo:cap.layer_check", RequiredTools: []string{"file_read"}, NewHandler: func(env agentenv.AgentContext) handler.InvocableCapabilityHandler {
		return NewLayerCheckHandler(env)
	}},
	{ID: "euclo:cap.targeted_refactor", RequiredTools: []string{"file_read", "file_write"}, NewHandler: func(env agentenv.AgentContext) handler.InvocableCapabilityHandler {
		return NewTargetedRefactorHandler(env)
	}},
	{ID: "euclo:cap.rename_symbol", RequiredTools: []string{"file_read", "file_write"}, NewHandler: func(env agentenv.AgentContext) handler.InvocableCapabilityHandler {
		return NewRenameSymbolHandler(env)
	}},
	{ID: "euclo:cap.api_compat", RequiredTools: []string{"file_read"}, NewHandler: func(env agentenv.AgentContext) handler.InvocableCapabilityHandler {
		return NewAPICompatHandler(env)
	}},
	{ID: "euclo:cap.boundary_report", RequiredTools: []string{"file_read"}, NewHandler: func(env agentenv.AgentContext) handler.InvocableCapabilityHandler {
		return NewBoundaryReportHandler(env)
	}},
	{ID: "euclo:cap.coverage_check", RequiredTools: []string{"file_read"}, NewHandler: func(env agentenv.AgentContext) handler.InvocableCapabilityHandler {
		return NewCoverageCheckHandler(env)
	}},
}

// AllCapabilityIDs returns the canonical list of euclo:cap.* IDs defined by
// the blueprint table. Callers use this to verify declared lists stay in sync.
func AllCapabilityIDs() []string {
	ids := make([]string, len(eucloRelurpicCapabilityBlueprints))
	for i, bp := range eucloRelurpicCapabilityBlueprints {
		ids[i] = bp.ID
	}
	return ids
}

// RegisterAll registers only the Euclo relurpic capability handlers declared
// by the active agent spec.
func RegisterAll(env agentenv.AgentContext, declared []string) error {
	if env.Registry == nil {
		return fmt.Errorf("capability registry is nil")
	}
	if len(declared) == 0 {
		return fmt.Errorf("capabilities.relurpic required")
	}

	declaredSet := make(map[string]struct{}, len(declared))
	for _, id := range declared {
		id = strings.TrimSpace(id)
		if id == "" {
			return fmt.Errorf("capabilities.relurpic contains empty capability id")
		}
		declaredSet[id] = struct{}{}
	}

	seen := make(map[string]struct{}, len(declaredSet))
	for _, blueprint := range eucloRelurpicCapabilityBlueprints {
		if _, ok := declaredSet[blueprint.ID]; !ok {
			continue
		}
		if env.Registry.HasCapability(blueprint.ID) {
			seen[blueprint.ID] = struct{}{}
			continue
		}
		handler := blueprint.NewHandler(env)
		if handler == nil {
			return fmt.Errorf("relurpic capability %s handler is nil", blueprint.ID)
		}
		if err := registerRelurpicCapability(env.Registry, relurpicCapabilitySpec{
			Handler:       handler,
			RequiredTools: blueprint.RequiredTools,
		}); err != nil {
			return fmt.Errorf("failed to register %s: %w", blueprint.ID, err)
		}
		seen[blueprint.ID] = struct{}{}
	}

	missing := make([]string, 0, len(declaredSet))
	for id := range declaredSet {
		if _, ok := seen[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("unknown relurpic capability declaration(s): %s", strings.Join(missing, ", "))
	}

	return nil
}
