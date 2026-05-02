package prompt

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Registry is the workspace-scoped prompt registry.
type Registry interface {
	// — Loading (called from ayenitd) —
	LoadDir(dir string) error
	LoadFS(fsys fs.FS, prefix string) error

	// — Provider registration (called from named-agent Initialize()) —
	// Returns error on duplicate name. Use IsAlreadyRegistered to check.
	RegisterProvider(name string, p ContextProvider) error

	// — Deferred provider validation —
	// Called once by ayenitd after all agents have completed Initialize().
	ValidateProviders() []ValidationIssue

	// — Lookup —
	Get(id string) (*PromptConfig, bool)
	All() []*PromptConfig
	Filter(opts FilterOptions) []*PromptConfig
	Count() int

	// — Resolution —
	Resolve(id string, ctx RuntimeContext) (string, error)
	ResolveDryRun(id string, ctx RuntimeContext) (DryRunResult, error)

	// — Introspection —
	DependsOn(id string) ([]string, error)
	DependentsOf(id string) ([]string, error)
	PromptVariables(id string) (map[string]VariableDecl, error)
	Validate(id string) []ValidationIssue
	ValidateAll() map[string][]ValidationIssue
}

// FilterOptions selects a subset of prompts from the registry.
type FilterOptions struct {
	Paradigm  string
	Agent     string
	Domain    string
	Kind      string
	Stability string
}

// DryRunResult is the output of ResolveDryRun.
type DryRunResult struct {
	Final          string
	BlocksIncluded []BlockTrace
	BlocksExcluded []BlockTrace
	Variables      map[string]string
	Warnings       []ValidationIssue
}

// BlockTrace records a block's participation in an assembly run.
type BlockTrace struct {
	BlockID string
	Source  BlockSource
	Order   int
	Reason  string
}

// NewRegistry returns a Registry with a no-op telemetry sink.
func NewRegistry() Registry {
	return newDefaultRegistry(noopTelemetry{})
}

// NewRegistryWithTelemetry returns a Registry using the provided telemetry sink.
func NewRegistryWithTelemetry(t PromptTelemetry) Registry {
	if t == nil {
		t = noopTelemetry{}
	}
	return newDefaultRegistry(t)
}

// defaultRegistry is the default Registry implementation.
type defaultRegistry struct {
	mu        sync.RWMutex
	prompts   map[string]*PromptConfig    // id → config
	fileOrder []string                    // ids in load order
	providers map[string]ContextProvider  // name → provider
	issues    map[string][]ValidationIssue // id → issues
	telemetry PromptTelemetry
}

func newDefaultRegistry(t PromptTelemetry) *defaultRegistry {
	return &defaultRegistry{
		prompts:   make(map[string]*PromptConfig),
		providers: make(map[string]ContextProvider),
		issues:    make(map[string][]ValidationIssue),
		telemetry: t,
	}
}

// LoadDir implements Registry. A missing directory is not an error.
func (r *defaultRegistry) LoadDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("prompt.LoadDir: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("prompt.LoadDir: %s is not a directory", dir)
	}

	var paths []string
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".prompt") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("prompt.LoadDir: walk %s: %w", dir, err)
	}

	return r.loadPaths(paths)
}

// LoadFS implements Registry using an io/fs.FS.
func (r *defaultRegistry) LoadFS(fsys fs.FS, prefix string) error {
	var results []*ParseResult
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".prompt") {
			return nil
		}
		data, readErr := fs.ReadFile(fsys, path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		sourcePath := path
		if prefix != "" {
			sourcePath = prefix + "/" + path
		}
		result, parseErr := ParseBytes(data, sourcePath)
		if parseErr != nil {
			r.telemetry.EmitPromptValidationIssue(ValidationIssueEvent{
				Issue: ValidationIssue{Severity: SeverityError, Message: parseErr.Error()},
			})
			return nil
		}
		results = append(results, result)
		return nil
	})
	if err != nil {
		return fmt.Errorf("prompt.LoadFS: walk: %w", err)
	}

	for _, result := range results {
		r.indexOne(result.Config, result.Warnings)
	}
	return r.pass2()
}

func (r *defaultRegistry) loadPaths(paths []string) error {
	var results []*ParseResult
	for _, p := range paths {
		result, err := ParseFile(p)
		if err != nil {
			r.telemetry.EmitPromptValidationIssue(ValidationIssueEvent{
				Issue: ValidationIssue{Severity: SeverityError,
					Message: fmt.Sprintf("parse %s: %v", p, err)},
			})
			continue
		}
		results = append(results, result)
	}

	for _, result := range results {
		r.indexOne(result.Config, result.Warnings)
	}
	return r.pass2()
}

// indexOne indexes a single parsed config (pass 1 — single-file validation).
func (r *defaultRegistry) indexOne(cfg *PromptConfig, warnings []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := cfg.ID
	if id == "" {
		r.telemetry.EmitPromptValidationIssue(ValidationIssueEvent{
			Issue: ValidationIssue{Severity: SeverityError,
				Message: "prompt file has no id: " + cfg.SourcePath},
		})
		return
	}

	if _, exists := r.prompts[id]; exists {
		iss := ValidationIssue{
			PromptID: id,
			Severity: SeverityError,
			Message:  "duplicate prompt id: " + id + " (" + cfg.SourcePath + ")",
		}
		r.issues[id] = append(r.issues[id], iss)
		r.telemetry.EmitPromptValidationIssue(ValidationIssueEvent{Issue: iss})
		return
	}

	for _, w := range warnings {
		r.issues[id] = append(r.issues[id], ValidationIssue{
			PromptID: id,
			Severity: SeverityWarning,
			Message:  w,
		})
	}

	structIssues := validateConfig(cfg)
	r.issues[id] = append(r.issues[id], structIssues...)
	for _, iss := range structIssues {
		if iss.Severity == SeverityError {
			r.telemetry.EmitPromptValidationIssue(ValidationIssueEvent{Issue: iss})
		}
	}

	if !hasErrors(r.issues[id]) {
		r.prompts[id] = cfg
		r.fileOrder = append(r.fileOrder, id)
	}
}

// pass2 resolves parent references and validates cross-prompt constraints.
func (r *defaultRegistry) pass2() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, cfg := range r.prompts {
		if cfg.Extends == "" {
			continue
		}
		parent, ok := r.prompts[cfg.Extends]
		if !ok {
			iss := ValidationIssue{
				PromptID: cfg.ID,
				Severity: SeverityError,
				Message:  "extends references unknown prompt: " + cfg.Extends,
			}
			r.issues[cfg.ID] = append(r.issues[cfg.ID], iss)
			r.telemetry.EmitPromptValidationIssue(ValidationIssueEvent{Issue: iss})
			continue
		}
		cfg.ParentResolved = parent
	}

	for _, cfg := range r.prompts {
		if cfg.Extends == "" {
			continue
		}
		issues := validateInheritance(cfg)
		r.issues[cfg.ID] = append(r.issues[cfg.ID], issues...)
		for _, iss := range issues {
			if iss.Severity == SeverityError {
				r.telemetry.EmitPromptValidationIssue(ValidationIssueEvent{Issue: iss})
			}
		}
	}

	return nil
}

// validateInheritance checks structural inheritance constraints.
func validateInheritance(cfg *PromptConfig) []ValidationIssue {
	var issues []ValidationIssue
	seen := make(map[string]bool)
	depth := 0
	cur := cfg
	for cur.ParentResolved != nil {
		depth++
		if depth > maxInheritanceDepth {
			issues = append(issues, ValidationIssue{
				PromptID: cfg.ID,
				Severity: SeverityError,
				Message:  "inheritance depth exceeds limit of 8",
			})
			return issues
		}
		if seen[cur.ParentResolved.ID] {
			issues = append(issues, ValidationIssue{
				PromptID: cfg.ID,
				Severity: SeverityError,
				Message:  "circular inheritance: " + cur.ParentResolved.ID,
			})
			return issues
		}
		seen[cur.ParentResolved.ID] = true
		cur = cur.ParentResolved
	}

	if cfg.ParentResolved != nil && len(cfg.Tags.Paradigm) > 0 && len(cfg.ParentResolved.Tags.Paradigm) > 0 {
		parentSet := make(map[string]bool)
		for _, p := range cfg.ParentResolved.Tags.Paradigm {
			parentSet[p] = true
		}
		for _, p := range cfg.Tags.Paradigm {
			if !parentSet[p] {
				issues = append(issues, ValidationIssue{
					PromptID: cfg.ID,
					Severity: SeverityError,
					Message:  "paradigm tag broadens parent's: " + p,
				})
			}
		}
	}

	return issues
}

// RegisterProvider implements Registry.
func (r *defaultRegistry) RegisterProvider(name string, p ContextProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[name]; exists {
		return &alreadyRegisteredError{Name: name}
	}
	r.providers[name] = p
	return nil
}

// ValidateProviders implements Registry. Called after all agents have initialized.
func (r *defaultRegistry) ValidateProviders() []ValidationIssue {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []ValidationIssue
	for _, cfg := range r.prompts {
		for _, name := range cfg.RequiresProviders {
			if _, ok := r.providers[name]; !ok {
				iss := ValidationIssue{
					PromptID: cfg.ID,
					Severity: SeverityError,
					Message:  "required provider not registered: " + name,
				}
				all = append(all, iss)
				r.telemetry.EmitPromptValidationIssue(ValidationIssueEvent{Issue: iss})
			}
		}
		// Warn about provider blocks not declared in requires_providers.
		required := make(map[string]bool, len(cfg.RequiresProviders))
		for _, n := range cfg.RequiresProviders {
			required[n] = true
		}
		for _, b := range cfg.Blocks {
			if b.From == SourceProvider && b.Provider != "" && !required[b.Provider] {
				iss := ValidationIssue{
					PromptID: cfg.ID,
					BlockID:  b.ID,
					Severity: SeverityWarning,
					Message:  "block uses provider not in requires_providers: " + b.Provider,
				}
				all = append(all, iss)
				r.telemetry.EmitPromptValidationIssue(ValidationIssueEvent{Issue: iss})
			}
		}
	}
	return all
}

// Get implements Registry.
func (r *defaultRegistry) Get(id string) (*PromptConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.prompts[id]
	return cfg, ok
}

// All implements Registry. Returns prompts in load order.
func (r *defaultRegistry) All() []*PromptConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*PromptConfig, 0, len(r.fileOrder))
	for _, id := range r.fileOrder {
		if cfg, ok := r.prompts[id]; ok {
			out = append(out, cfg)
		}
	}
	return out
}

// Filter implements Registry.
func (r *defaultRegistry) Filter(opts FilterOptions) []*PromptConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*PromptConfig
	for _, id := range r.fileOrder {
		cfg, ok := r.prompts[id]
		if !ok {
			continue
		}
		if opts.Paradigm != "" && !containsString(cfg.Tags.Paradigm, opts.Paradigm) {
			continue
		}
		if opts.Agent != "" && !containsString(cfg.Tags.Agent, opts.Agent) {
			continue
		}
		if opts.Domain != "" && !containsString(cfg.Tags.Domain, opts.Domain) {
			continue
		}
		if opts.Kind != "" && cfg.Tags.Kind != opts.Kind {
			continue
		}
		if opts.Stability != "" && cfg.Tags.Stability != opts.Stability {
			continue
		}
		out = append(out, cfg)
	}
	return out
}

// Count implements Registry.
func (r *defaultRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.prompts)
}

// Resolve implements Registry.
func (r *defaultRegistry) Resolve(id string, ctx RuntimeContext) (string, error) {
	start := time.Now()

	r.mu.RLock()
	cfg, ok := r.prompts[id]
	providers := r.snapshotProviders()
	r.mu.RUnlock()

	if !ok {
		err := &NotFoundError{ID: id}
		r.telemetry.EmitPromptResolveFailed(ResolveFailedEvent{
			ID:       id,
			Paradigm: ctx.Paradigm,
			Error:    err.Error(),
		})
		return "", err
	}

	if ctx.Paradigm != "" && len(cfg.Tags.Paradigm) > 0 {
		if !containsString(cfg.Tags.Paradigm, ctx.Paradigm) {
			err := &ParadigmMismatchError{
				ID:       id,
				Required: cfg.Tags.Paradigm,
				Actual:   ctx.Paradigm,
			}
			r.telemetry.EmitPromptResolveFailed(ResolveFailedEvent{
				ID:         id,
				Paradigm:   ctx.Paradigm,
				Error:      err.Error(),
				DurationMs: time.Since(start).Milliseconds(),
			})
			return "", err
		}
	}

	result, included, excluded, err := assemble(cfg, ctx, providers)
	if err != nil {
		r.telemetry.EmitPromptResolveFailed(ResolveFailedEvent{
			ID:         id,
			Paradigm:   ctx.Paradigm,
			Error:      err.Error(),
			DurationMs: time.Since(start).Milliseconds(),
		})
		return "", fmt.Errorf("resolve %s: %w", id, err)
	}

	r.telemetry.EmitPromptResolved(ResolvedEvent{
		ID:             id,
		Paradigm:       ctx.Paradigm,
		OutputLength:   len(result),
		BlocksIncluded: len(included),
		BlocksExcluded: len(excluded),
		ProvidersUsed:  providerNamesFromTraces(included),
		DurationMs:     time.Since(start).Milliseconds(),
	})

	return result, nil
}

// ResolveDryRun implements Registry.
func (r *defaultRegistry) ResolveDryRun(id string, ctx RuntimeContext) (DryRunResult, error) {
	r.mu.RLock()
	cfg, ok := r.prompts[id]
	providers := r.snapshotProviders()
	r.mu.RUnlock()

	if !ok {
		return DryRunResult{}, &NotFoundError{ID: id}
	}

	result, included, excluded, err := assemble(cfg, ctx, providers)
	if err != nil {
		return DryRunResult{}, fmt.Errorf("dry-run %s: %w", id, err)
	}

	vars := mergeVariables(cfg)
	varMap := make(map[string]string, len(vars))
	for k, v := range vars {
		if rv, ok := ctx.Variables[k]; ok {
			varMap[k] = rv
		} else {
			varMap[k] = v.Default
		}
	}

	return DryRunResult{
		Final:          result,
		BlocksIncluded: included,
		BlocksExcluded: excluded,
		Variables:      varMap,
	}, nil
}

// DependsOn returns the chain of prompt IDs that id inherits from.
func (r *defaultRegistry) DependsOn(id string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.prompts[id]
	if !ok {
		return nil, &NotFoundError{ID: id}
	}
	var chain []string
	cur := cfg.ParentResolved
	for cur != nil {
		chain = append(chain, cur.ID)
		cur = cur.ParentResolved
	}
	return chain, nil
}

// DependentsOf returns the ids of prompts that extend id.
func (r *defaultRegistry) DependentsOf(id string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.prompts[id]; !ok {
		return nil, &NotFoundError{ID: id}
	}
	var deps []string
	for _, cfg := range r.prompts {
		if cfg.Extends == id {
			deps = append(deps, cfg.ID)
		}
	}
	return deps, nil
}

// PromptVariables returns the effective (inherited) variable declarations for id.
func (r *defaultRegistry) PromptVariables(id string) (map[string]VariableDecl, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.prompts[id]
	if !ok {
		return nil, &NotFoundError{ID: id}
	}
	return mergeVariables(cfg), nil
}

// Validate returns all load-time issues for id.
func (r *defaultRegistry) Validate(id string) []ValidationIssue {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]ValidationIssue{}, r.issues[id]...)
}

// ValidateAll returns all load-time issues across all prompts.
func (r *defaultRegistry) ValidateAll() map[string][]ValidationIssue {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string][]ValidationIssue, len(r.issues))
	for id, issues := range r.issues {
		out[id] = append([]ValidationIssue{}, issues...)
	}
	return out
}

// ---- helpers ----------------------------------------------------------------

// snapshotProviders returns a snapshot of the provider map (must be called with RLock held).
func (r *defaultRegistry) snapshotProviders() map[string]ContextProvider {
	snap := make(map[string]ContextProvider, len(r.providers))
	for k, v := range r.providers {
		snap[k] = v
	}
	return snap
}

func hasErrors(issues []ValidationIssue) bool {
	for _, iss := range issues {
		if iss.Severity == SeverityError {
			return true
		}
	}
	return false
}

func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func providerNamesFromTraces(traces []BlockTrace) []string {
	var names []string
	seen := make(map[string]bool)
	for _, t := range traces {
		if t.Source == SourceProvider && !seen[t.BlockID] {
			names = append(names, t.BlockID)
			seen[t.BlockID] = true
		}
	}
	return names
}
