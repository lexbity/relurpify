package relurpicabilities

import (
	"fmt"
	"os"
	"sort"
	"strings"

	registry "codeburg.org/lexbit/relurpify/capability/registry"

	"codeburg.org/lexbit/relurpify/capability/handler"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/model"
)

// RegistrationDeps is the dependency contract for RegisterAll.
// All fields except Registry and Declared are optional; handlers degrade
// gracefully to "service not available" when their deps are nil.
type RegistrationDeps struct {
	Registry *registry.CapabilityRegistry
	Declared []string

	// IndexManager provides AST symbol queries, node search, call/dependency
	// graph reading, and index refresh after file mutations. It also exposes
	// the backing IndexStore (cast to *ast.GraphIndexStore) for edge lookups.
	IndexManager *ast.IndexManager

	// Workspace is the absolute path to the workspace root. Required by
	// WorkspaceFiles-backed handlers (targeted_refactor, rename_symbol).
	Workspace string

	// CommandRunner is the policy-wrapped shell executor. Required by
	// command-backed handlers (test_run, blame_trace, bisect, diff_summary,
	// api_compat, coverage_check).
	CommandRunner CommandRuntime

	// CommandPolicy controls whether a command is allowed to execute.
	CommandPolicy CommandPolicy

	// Model is the language model for LLM-backed handlers (code_review,
	// diff_summary, bisect).
	Model model.LanguageModel
}

// indexDepsFromRegistration builds an IndexDeps from the registration deps.
// Returns a zero-value IndexDeps when IndexManager is nil.
func indexDepsFromRegistration(deps RegistrationDeps) IndexDeps {
	d := IndexDeps{Workspace: deps.Workspace}
	if deps.IndexManager == nil {
		return d
	}
	d.Searcher = deps.IndexManager
	d.Grapher = deps.IndexManager
	// Expose the backing store for edge lookups when it is a GraphIndexStore.
	if store, ok := deps.IndexManager.Store().(*ast.GraphIndexStore); ok {
		d.Store = store
	}
	return d
}

// commandDepsFromRegistration builds a CommandDeps from the registration deps.
func commandDepsFromRegistration(deps RegistrationDeps) CommandDeps {
	return CommandDeps{
		Runner:    deps.CommandRunner,
		Policy:    deps.CommandPolicy,
		Workspace: deps.Workspace,
	}
}

// workspaceFilesFromRegistration returns a WorkspaceFiles backed by the real OS
// filesystem rooted at deps.Workspace, or nil when Workspace is empty.
func workspaceFilesFromRegistration(deps RegistrationDeps) WorkspaceFiles {
	if deps.Workspace == "" {
		return nil
	}
	return &workspaceFileSystem{workspace: deps.Workspace}
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
		return NewTestRunHandler(commandDepsFromRegistration(deps))
	}},
	{ID: "euclo:cap.ast_query", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		idx := indexDepsFromRegistration(deps)
		return NewASTQueryHandler(idx.Searcher)
	}},
	{ID: "euclo:cap.symbol_trace", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		idx := indexDepsFromRegistration(deps)
		return NewSymbolTraceHandler(idx.Grapher)
	}},
	{ID: "euclo:cap.call_graph", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewCallGraphHandler(indexDepsFromRegistration(deps))
	}},
	{ID: "euclo:cap.blame_trace", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		// blame_trace uses its own symbolResolver interface; pass nil — symbol
		// line-range is optional and degrades gracefully.
		return NewBlameTraceHandler(commandDepsFromRegistration(deps), nil)
	}},
	{ID: "euclo:cap.bisect", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewBisectHandler(commandDepsFromRegistration(deps), deps.Model)
	}},
	{ID: "euclo:cap.code_review", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewCodeReviewHandler(deps.Model, deps.Registry, nil)
	}},
	{ID: "euclo:cap.diff_summary", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewDiffSummaryHandler(commandDepsFromRegistration(deps), deps.Model)
	}},
	{ID: "euclo:cap.layer_check", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewLayerCheckHandler(indexDepsFromRegistration(deps))
	}},
	{ID: "euclo:cap.targeted_refactor", RequiredTools: []string{"file_read", "file_write"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		idx := indexDepsFromRegistration(deps)
		// IndexManager implements both SymbolQuerier (QuerySymbol) and IndexRefresher (RefreshFiles).
		return NewTargetedRefactorHandler(deps.IndexManager, idx.Store, workspaceFilesFromRegistration(deps), deps.IndexManager, deps.Model)
	}},
	{ID: "euclo:cap.rename_symbol", RequiredTools: []string{"file_read", "file_write"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		idx := indexDepsFromRegistration(deps)
		// IndexManager implements SymbolQuerier (QuerySymbol) and IndexRefresher (RefreshFiles).
		return NewRenameSymbolHandler(deps.IndexManager, idx.Store, workspaceFilesFromRegistration(deps), deps.IndexManager)
	}},
	{ID: "euclo:cap.api_compat", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewAPICompatHandler(commandDepsFromRegistration(deps))
	}},
	{ID: "euclo:cap.boundary_report", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewBoundaryReportHandler(indexDepsFromRegistration(deps))
	}},
	{ID: "euclo:cap.coverage_check", RequiredTools: []string{"file_read"}, NewHandler: func(deps RegistrationDeps) handler.InvocableCapabilityHandler {
		return NewCoverageCheckHandler(commandDepsFromRegistration(deps))
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
