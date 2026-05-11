package prompt

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Registry is the workspace-scoped prompt registry.
type Registry interface {
	LoadDir(dir string) error
	LoadFS(fsys fs.FS, prefix string) error
	RegisterProvider(name string, p ContextProvider) error
	ValidateProviders() []ValidationIssue
	Get(id string) (*PromptConfig, bool)
	All() []*PromptConfig
	Filter(opts FilterOptions) []*PromptConfig
	Count() int
	Resolve(id string, ctx RuntimeContext) (string, error)
	ResolveDryRun(id string, ctx RuntimeContext) (DryRunResult, error)
	DependsOn(id string) ([]string, error)
	DependentsOf(id string) ([]string, error)
	PromptVariables(id string) (map[string]VariableDecl, error)
	Validate(id string) []ValidationIssue
	ValidateAll() map[string][]ValidationIssue
}

// FilterOptions selects a subset of prompts from the registry.
type FilterOptions struct {
	Tags []string
}

// DryRunResult is the output of ResolveDryRun.
type DryRunResult struct {
	Final          string
	BlocksIncluded []BlockTrace
	BlocksExcluded []BlockTrace
	Variables      map[string]string
	Warnings       []ValidationIssue
}

// BlockTrace is retained for resolution diagnostics.
type BlockTrace struct {
	BlockID string
	Source  string
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

type defaultRegistry struct {
	mu        sync.RWMutex
	prompts   map[string]*PromptConfig
	fileOrder []string
	providers map[string]ContextProvider
	issues    map[string][]ValidationIssue
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
	sort.Strings(paths)
	return r.loadPaths(paths)
}

func (r *defaultRegistry) LoadFS(fsys fs.FS, prefix string) error {
	var paths []string
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".prompt") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return fmt.Errorf("prompt.LoadFS: walk: %w", err)
	}
	sort.Strings(paths)

	for _, path := range paths {
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
			issue := ValidationIssue{Severity: SeverityError, Message: parseErr.Error()}
			r.telemetry.EmitPromptValidationIssue(ValidationIssueEvent{Issue: issue})
			return parseErr
		}
		if err := r.indexOne(result.Config, result.Warnings); err != nil {
			return err
		}
	}
	return nil
}

func (r *defaultRegistry) loadPaths(paths []string) error {
	for _, path := range paths {
		result, err := ParseFile(path)
		if err != nil {
			issue := ValidationIssue{Severity: SeverityError, Message: fmt.Sprintf("parse %s: %v", path, err)}
			r.telemetry.EmitPromptValidationIssue(ValidationIssueEvent{Issue: issue})
			return err
		}
		if err := r.indexOne(result.Config, result.Warnings); err != nil {
			return err
		}
	}
	return nil
}

func (r *defaultRegistry) indexOne(cfg *PromptConfig, warnings []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cfg == nil {
		return &ValidationError{Issues: []ValidationIssue{{Severity: SeverityError, Message: "prompt config is nil"}}}
	}
	if cfg.ID == "" {
		iss := ValidationIssue{Severity: SeverityError, Message: "prompt file has no id: " + cfg.SourcePath}
		r.issues[""] = append(r.issues[""], iss)
		r.telemetry.EmitPromptValidationIssue(ValidationIssueEvent{Issue: iss})
		return &ValidationError{Issues: []ValidationIssue{iss}}
	}
	if existing, exists := r.prompts[cfg.ID]; exists {
		iss := ValidationIssue{PromptID: cfg.ID, Severity: SeverityError, Message: "duplicate prompt id: " + cfg.ID + " (" + existing.SourcePath + ", " + cfg.SourcePath + ")"}
		r.issues[cfg.ID] = append(r.issues[cfg.ID], iss)
		r.telemetry.EmitPromptValidationIssue(ValidationIssueEvent{Issue: iss})
		return &DuplicateIDError{ID: cfg.ID, ExistingPath: existing.SourcePath, NewPath: cfg.SourcePath}
	}

	for _, w := range warnings {
		iss := ValidationIssue{PromptID: cfg.ID, Severity: SeverityWarning, Message: w}
		r.issues[cfg.ID] = append(r.issues[cfg.ID], iss)
		r.telemetry.EmitPromptValidationIssue(ValidationIssueEvent{Issue: iss})
	}

	structIssues := validateConfig(cfg)
	r.issues[cfg.ID] = append(r.issues[cfg.ID], structIssues...)
	for _, iss := range structIssues {
		if iss.Severity == SeverityError {
			r.telemetry.EmitPromptValidationIssue(ValidationIssueEvent{Issue: iss})
		}
	}

	if !hasErrors(r.issues[cfg.ID]) {
		r.prompts[cfg.ID] = cfg
		r.fileOrder = append(r.fileOrder, cfg.ID)
	}
	if hasErrors(structIssues) {
		return &ValidationError{Issues: structIssues}
	}
	return nil
}

func (r *defaultRegistry) RegisterProvider(name string, p ContextProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[name]; exists {
		return &alreadyRegisteredError{Name: name}
	}
	r.providers[name] = p
	return nil
}

func (r *defaultRegistry) ValidateProviders() []ValidationIssue { return nil }

func (r *defaultRegistry) Get(id string) (*PromptConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.prompts[id]
	return cfg, ok
}

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

func (r *defaultRegistry) Filter(opts FilterOptions) []*PromptConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(opts.Tags) == 0 {
		out := make([]*PromptConfig, 0, len(r.fileOrder))
		for _, id := range r.fileOrder {
			if cfg, ok := r.prompts[id]; ok {
				out = append(out, cfg)
			}
		}
		return out
	}
	var out []*PromptConfig
	for _, id := range r.fileOrder {
		cfg, ok := r.prompts[id]
		if !ok {
			continue
		}
		if !hasAnyTag(cfg.Tags, opts.Tags) {
			continue
		}
		out = append(out, cfg)
	}
	return out
}

func (r *defaultRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.prompts)
}

func (r *defaultRegistry) Resolve(id string, ctx RuntimeContext) (string, error) {
	start := time.Now()

	r.mu.RLock()
	cfg, ok := r.prompts[id]
	r.mu.RUnlock()
	if !ok {
		err := &NotFoundError{ID: id}
		r.telemetry.EmitPromptResolveFailed(ResolveFailedEvent{ID: id, Paradigm: ctx.Paradigm, Error: err.Error()})
		return "", err
	}

	result, _, err := resolvePrompt(cfg, ctx)
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
		ID:           id,
		Paradigm:     ctx.Paradigm,
		OutputLength: len(result),
		DurationMs:   time.Since(start).Milliseconds(),
	})
	return result, nil
}

func (r *defaultRegistry) ResolveDryRun(id string, ctx RuntimeContext) (DryRunResult, error) {
	r.mu.RLock()
	cfg, ok := r.prompts[id]
	r.mu.RUnlock()
	if !ok {
		return DryRunResult{}, &NotFoundError{ID: id}
	}
	result, vars, err := resolvePrompt(cfg, ctx)
	if err != nil {
		return DryRunResult{}, fmt.Errorf("dry-run %s: %w", id, err)
	}
	return DryRunResult{Final: result, Variables: vars}, nil
}

func (r *defaultRegistry) DependsOn(string) ([]string, error) { return nil, nil }
func (r *defaultRegistry) DependentsOf(string) ([]string, error) {
	return nil, nil
}

func (r *defaultRegistry) PromptVariables(id string) (map[string]VariableDecl, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.prompts[id]
	if !ok {
		return nil, &NotFoundError{ID: id}
	}
	out := make(map[string]VariableDecl, len(cfg.Variables))
	for name, decl := range cfg.Variables {
		out[name] = decl
	}
	return out, nil
}

func (r *defaultRegistry) Validate(id string) []ValidationIssue {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]ValidationIssue(nil), r.issues[id]...)
}

func (r *defaultRegistry) ValidateAll() map[string][]ValidationIssue {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string][]ValidationIssue, len(r.issues))
	for id, iss := range r.issues {
		out[id] = append([]ValidationIssue(nil), iss...)
	}
	return out
}

func hasAnyTag(tags, filter []string) bool {
	set := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		set[tag] = struct{}{}
	}
	for _, tag := range filter {
		if _, ok := set[tag]; ok {
			return true
		}
	}
	return false
}

func hasErrors(issues []ValidationIssue) bool {
	for _, iss := range issues {
		if iss.Severity == SeverityError {
			return true
		}
	}
	return false
}
