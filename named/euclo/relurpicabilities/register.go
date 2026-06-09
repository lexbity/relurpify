package relurpicabilities

import (
	"fmt"
	"os"
	"sort"
	"strings"

	registry "codeburg.org/lexbit/relurpify/capability/registry"

	"codeburg.org/lexbit/relurpify/capability/handler"
)

// RegistrationDeps is the dependency contract for RegisterAll.
type RegistrationDeps struct {
	Registry *registry.CapabilityRegistry
	Declared []string
}

type relurpicCapabilityBlueprint struct {
	ID            string
	RequiredTools []string
	NewHandler    func(RegistrationDeps) handler.InvocableCapabilityHandler
}

// workspaceFileSystem implements WorkspaceFiles using the OS filesystem.
type workspaceFileSystem struct {
	workspace string
}

func (w *workspaceFileSystem) Resolve(candidate string) (string, string, error) {
	return resolveCandidatePath(candidate, w.workspace), resolveCandidatePath(candidate, w.workspace), nil
}

func (w *workspaceFileSystem) Read(candidate string) ([]byte, string, error) {
	resolved := resolveCandidatePath(candidate, w.workspace)
	if resolved == "" {
		return nil, "", fmt.Errorf("path resolution failed: %s", candidate)
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return nil, "", err
	}
	return content, resolved, nil
}

func (w *workspaceFileSystem) Write(candidate string, content []byte, perm os.FileMode) (string, error) {
	resolved := resolveCandidatePath(candidate, w.workspace)
	if resolved == "" {
		return "", fmt.Errorf("path resolution failed: %s", candidate)
	}
	if err := os.WriteFile(resolved, content, perm); err != nil {
		return "", err
	}
	return resolved, nil
}

var eucloRelurpicCapabilityBlueprints = []relurpicCapabilityBlueprint{
	{ID: "euclo:cap.test_run", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewTestRunHandler(CommandDeps{})
	}},
	{ID: "euclo:cap.ast_query", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewASTQueryHandler(nil)
	}},
	{ID: "euclo:cap.symbol_trace", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewSymbolTraceHandler(nil)
	}},
	{ID: "euclo:cap.call_graph", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewCallGraphHandler(IndexDeps{})
	}},
	{ID: "euclo:cap.blame_trace", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewBlameTraceHandler(CommandDeps{}, nil)
	}},
	{ID: "euclo:cap.bisect", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewBisectHandler(CommandDeps{}, nil)
	}},
	{ID: "euclo:cap.code_review", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewCodeReviewHandler(nil, deps.Registry, nil)
	}},
	{ID: "euclo:cap.diff_summary", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewDiffSummaryHandler(CommandDeps{}, nil)
	}},
	{ID: "euclo:cap.layer_check", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewLayerCheckHandler(IndexDeps{})
	}},
	{ID: "euclo:cap.targeted_refactor", RequiredTools: []string{"file_read", "file_write"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewTargetedRefactorHandler(nil, nil, nil, nil, nil)
	}},
	{ID: "euclo:cap.rename_symbol", RequiredTools: []string{"file_read", "file_write"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewRenameSymbolHandler(nil, nil, nil, nil)
	}},
	{ID: "euclo:cap.api_compat", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewAPICompatHandler(CommandDeps{})
	}},
	{ID: "euclo:cap.boundary_report", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewBoundaryReportHandler(IndexDeps{})
	}},
	{ID: "euclo:cap.coverage_check", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewCoverageCheckHandler(CommandDeps{})
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

// RegisterAll registers relurpic capability handlers declared in deps.Declared.
// It is idempotent: already-registered capabilities are skipped without error.
// Unknown declared IDs cause a failure.
func RegisterAll(deps RegistrationDeps) error {
	if deps.Registry == nil {
		return fmt.Errorf("capability registry is nil")
	}
	if len(deps.Declared) == 0 {
		return fmt.Errorf("capabilities.relurpic required")
	}

	declaredSet := normalizeDeclared(deps.Declared)

	seen := make(map[string]struct{}, len(declaredSet))
	for _, blueprint := range eucloRelurpicCapabilityBlueprints {
		if _, ok := declaredSet[blueprint.ID]; !ok {
			continue
		}
		if deps.Registry.HasCapability(blueprint.ID) {
			seen[blueprint.ID] = struct{}{}
			continue
		}
		handler := blueprint.NewHandler(deps)
		if handler == nil {
			return fmt.Errorf("relurpic capability %s handler is nil", blueprint.ID)
		}
		if err := registerRelurpicCapability(deps.Registry, relurpicCapabilitySpec{
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

// normalizeDeclared deduplicates and validates declared capability IDs.
func normalizeDeclared(ids []string) map[string]struct{} {
	seen := make(map[string]struct{}, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		seen[id] = struct{}{}
	}
	return seen
}
