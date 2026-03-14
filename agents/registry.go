package agents

import (
	"errors"
	"fmt"
	frameworkconfig "github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RegistryOptions configures the agent discovery behavior.
type RegistryOptions struct {
	Workspace string
	Paths     []string
	RulesPath string
}

// Registry tracks loaded manifests and supports hot reloading.
type Registry struct {
	opts    RegistryOptions
	mu      sync.RWMutex
	agents  map[string]*manifest.AgentManifest
	errors  map[string]string
	watchCh []chan struct{}
	rules   *Ruleset
	loaded  time.Time
}

// NewRegistry builds an empty registry.
func NewRegistry(opts RegistryOptions) *Registry {
	return &Registry{
		opts:   opts,
		agents: make(map[string]*manifest.AgentManifest),
		errors: make(map[string]string),
	}
}

// Load scans the configured directories for manifests.
func (r *Registry) Load() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents = make(map[string]*manifest.AgentManifest)
	r.errors = make(map[string]string)
	for _, dir := range r.searchPaths() {
		r.loadPath(dir)
	}
	if r.opts.RulesPath != "" {
		if rules, err := LoadRuleset(r.opts.RulesPath); err == nil {
			r.rules = rules
		}
	}
	r.loaded = time.Now()
	r.broadcast()
	return nil
}

// Reload rescans the filesystem and notifies subscribers.
func (r *Registry) Reload() error {
	return r.Load()
}

// List returns summaries of available agents.
func (r *Registry) List() []AgentSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()
	summaries := make([]AgentSummary, 0, len(r.agents))
	for _, manifest := range r.agents {
		summaries = append(summaries, summarizeManifest(manifest, r.opts.Workspace))
	}
	return summaries
}

// Errors returns manifest load errors keyed by source path.
func (r *Registry) Errors() []RegistryLoadError {
	r.mu.RLock()
	defer r.mu.RUnlock()
	errs := make([]RegistryLoadError, 0, len(r.errors))
	for path, message := range r.errors {
		errs = append(errs, RegistryLoadError{Path: path, Error: message})
	}
	return errs
}

// Get retrieves a manifest by name.
func (r *Registry) Get(name string) (*manifest.AgentManifest, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	manifest, ok := r.agents[name]
	return manifest, ok
}

// Rules returns the project ruleset when available.
func (r *Registry) Rules() *Ruleset {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.rules
}

// Watch registers a listener notified on reload events.
func (r *Registry) Watch() <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch := make(chan struct{}, 1)
	r.watchCh = append(r.watchCh, ch)
	return ch
}

// broadcast notifies every watcher that the registry contents changed.
func (r *Registry) broadcast() {
	for _, ch := range r.watchCh {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// loadPath loads a manifest file or every manifest inside a directory.
func (r *Registry) loadPath(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if !info.IsDir() {
		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			return
		}
		if manifest, err := manifest.LoadAgentManifest(path); err == nil {
			r.agents[manifest.Metadata.Name] = manifest
		} else {
			r.recordLoadError(path, err)
		}
		return
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		entryPath := filepath.Join(path, entry.Name())
		if manifest, err := manifest.LoadAgentManifest(entryPath); err == nil {
			r.agents[manifest.Metadata.Name] = manifest
		} else {
			r.recordLoadError(entryPath, err)
		}
	}
}

// searchPaths resolves configured agent paths, de-duplicating them and
// expanding workspace-relative aliases.
func (r *Registry) searchPaths() []string {
	paths := r.opts.Paths
	if len(paths) == 0 {
		paths = DefaultAgentPaths(r.opts.Workspace)
	}
	set := make(map[string]struct{})
	var resolved []string
	for _, path := range paths {
		path = expandPath(path, r.opts.Workspace)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if _, exists := set[path]; exists {
			continue
		}
		set[path] = struct{}{}
		resolved = append(resolved, path)
	}
	return resolved
}

// StartWatcher polls for filesystem changes.
func (r *Registry) StartWatcher(stop <-chan struct{}, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		last := time.Time{}
		for {
			select {
			case <-ticker.C:
				info := r.snapshot()
				if info.After(last) {
					_ = r.Load()
					last = info
				}
			case <-stop:
				return
			}
		}
	}()
}

// snapshot walks the agent directories and returns the newest modification
// timestamp. The result lets StartWatcher detect changes without storing full
// directory listings.
func (r *Registry) snapshot() time.Time {
	paths := r.searchPaths()
	var newest time.Time
	for _, path := range paths {
		filepath.WalkDir(path, func(current string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if ts := fetchModTime(current); ts.After(newest) {
				newest = ts
			}
			return nil
		})
	}
	return newest
}

// RegistryLoadError represents a manifest load failure.
type RegistryLoadError struct {
	Path  string
	Error string
}

func (r *Registry) recordLoadError(path string, err error) {
	if r == nil || err == nil {
		return
	}
	r.errors[path] = err.Error()
	logRegistryError(r.opts.Workspace, path, err)
}

func logRegistryError(workspace, path string, err error) {
	if workspace == "" {
		return
	}
	logDir := filepath.Join(frameworkconfig.New(workspace).ConfigRoot(), "logs")
	if mkErr := os.MkdirAll(logDir, 0o755); mkErr != nil {
		return
	}
	file := filepath.Join(logDir, "agent_registry.log")
	entry := fmt.Sprintf("%s load error for %s: %s\n", time.Now().UTC().Format(time.RFC3339), path, err.Error())
	if f, openErr := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); openErr == nil {
		defer f.Close()
		_, _ = f.WriteString(entry)
	}
}

// fetchModTime retrieves the modification time, returning the zero time on
// errors so callers can ignore missing files.
func fetchModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// ErrAgentNotFound indicates lookup failure.
var ErrAgentNotFound = errors.New("agent not found")

// AgentSummary is a lightweight view of available manifests.
type AgentSummary struct {
	Name        string
	Description string
	Mode        core.AgentMode
	Model       string
	Source      string
}

// summarizeManifest converts the manifest metadata into a lightweight CLI view.
func summarizeManifest(m *manifest.AgentManifest, workspace string) AgentSummary {
	source := m.SourcePath
	if workspace != "" {
		if rel, err := filepath.Rel(workspace, source); err == nil {
			source = rel
		}
	}
	var mode core.AgentMode
	var modelName string
	if m.Spec.Agent != nil {
		mode = m.Spec.Agent.Mode
		modelName = m.Spec.Agent.Model.Name
	}
	return AgentSummary{
		Name:        m.Metadata.Name,
		Description: m.Metadata.Description,
		Mode:        mode,
		Model:       modelName,
		Source:      source,
	}
}
