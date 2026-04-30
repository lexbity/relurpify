package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkmanifest "codeburg.org/lexbit/relurpify/framework/manifest"
)

// ensureWorkspace resolves the workspace CLI flag, defaulting to cwd.
func ensureWorkspace() string {
	if workspace == "" {
		wd, _ := os.Getwd()
		workspace = wd
	}
	return workspace
}

type agentSummary struct {
	Name        string
	Mode        string
	Model       string
	Source      string
	Description string
}

type agentLoadError struct {
	Path  string
	Error string
}

type agentRegistry struct {
	manifests map[string]*frameworkmanifest.AgentManifest
	summaries []agentSummary
	errs      []agentLoadError
}

func newAgentRegistry() *agentRegistry {
	return &agentRegistry{manifests: map[string]*frameworkmanifest.AgentManifest{}}
}

func (r *agentRegistry) Get(name string) (*frameworkmanifest.AgentManifest, bool) {
	if r == nil {
		return nil, false
	}
	m, ok := r.manifests[name]
	return m, ok
}

func (r *agentRegistry) List() []agentSummary {
	if r == nil {
		return nil
	}
	out := make([]agentSummary, len(r.summaries))
	copy(out, r.summaries)
	return out
}

func (r *agentRegistry) Errors() []agentLoadError {
	if r == nil {
		return nil
	}
	out := make([]agentLoadError, len(r.errs))
	copy(out, r.errs)
	return out
}

// buildRegistry loads manifests scoped to the workspace.
func buildRegistry(workspace string) (*agentRegistry, error) {
	reg := newAgentRegistry()
	paths := frameworkmanifest.DefaultAgentPaths(workspace)
	if globalCfg != nil {
		paths = globalCfg.AgentSearchPaths(workspace)
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			entries, err := os.ReadDir(path)
			if err != nil {
				reg.errs = append(reg.errs, agentLoadError{Path: path, Error: err.Error()})
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
					continue
				}
				reg.load(filepath.Join(path, entry.Name()))
			}
			continue
		}
		reg.load(path)
	}
	sort.SliceStable(reg.summaries, func(i, j int) bool {
		return reg.summaries[i].Name < reg.summaries[j].Name
	})
	return reg, nil
}

func (r *agentRegistry) load(path string) {
	loaded, err := frameworkmanifest.LoadAgentManifest(path)
	if err != nil {
		r.errs = append(r.errs, agentLoadError{Path: path, Error: err.Error()})
		return
	}
	r.manifests[loaded.Metadata.Name] = loaded
	summary := agentSummary{
		Name:        loaded.Metadata.Name,
		Description: loaded.Metadata.Description,
		Source:      path,
	}
	if loaded.Spec.Agent != nil {
		summary.Mode = string(loaded.Spec.Agent.Mode)
		summary.Model = loaded.Spec.Agent.Model.Name
	}
	r.summaries = append(r.summaries, summary)
}

func effectiveAgentSpec(m *frameworkmanifest.AgentManifest, contract *frameworkmanifest.EffectiveAgentContract) *core.AgentRuntimeSpec {
	if contract != nil {
		return contract.AgentSpec
	}
	if m == nil {
		return nil
	}
	return m.Spec.Agent
}

// readConfigMap deserializes manifest.yaml into a generic map for dotted lookups.
func readConfigMap(path string) (map[string]interface{}, error) {
	data := map[string]interface{}{}
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return data, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(bytes, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// writeConfigMap persists the config map back to YAML, creating directories.
func writeConfigMap(path string, data map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	bytes, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0o644)
}

// getConfigValue traverses a nested map using dotted notation.
func getConfigValue(data map[string]interface{}, key string) (interface{}, bool) {
	parts := strings.Split(key, ".")
	var current interface{} = data
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		value, ok := m[part]
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}

// setConfigValue mutates/creates nested keys referenced via dotted notation.
func setConfigValue(data map[string]interface{}, key string, value interface{}) error {
	parts := strings.Split(key, ".")
	current := data
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return nil
		}
		next, ok := current[part].(map[string]interface{})
		if !ok {
			next = map[string]interface{}{}
			current[part] = next
		}
		current = next
	}
	return nil
}

// parseValue attempts to coerce CLI input into bool/int/float before storing.
func parseValue(input string) interface{} {
	if b, err := strconv.ParseBool(input); err == nil {
		return b
	}
	if i, err := strconv.ParseInt(input, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(input, 64); err == nil {
		return f
	}
	return input
}

// prettyValue renders nested values in a human-readable one-line format.
func prettyValue(v interface{}) string {
	switch value := v.(type) {
	case []interface{}:
		var parts []string
		for _, item := range value {
			parts = append(parts, prettyValue(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]interface{}:
		b, _ := yaml.Marshal(value)
		return strings.TrimSpace(string(b))
	default:
		return fmt.Sprint(value)
	}
}

// sessionDir returns the path where session yaml files live.
func sessionDir() string {
	return filepath.Join(frameworkmanifest.New(ensureWorkspace()).ConfigRoot(), "sessions")
}

// sanitizeName normalizes user-provided identifiers for filenames.
func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "_")
	return name
}

// stringValue renders arbitrary values as trimmed strings for CLI helpers.
func stringValue(v any) string {
	switch typed := v.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

// uniqueStrings removes duplicates while preserving the first seen order.
func uniqueStrings(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(input))
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

// firstNonEmpty returns the first trimmed non-empty string from the inputs.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// containsString reports whether target exists in items.
func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
