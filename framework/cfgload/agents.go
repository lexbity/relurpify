package cfgload

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/cfgload/model"
)

// FilesystemAction is a validated filesystem permission action.
type FilesystemAction string

const (
	FSRead    FilesystemAction = "fs:read"
	FSList    FilesystemAction = "fs:list"
	FSWrite   FilesystemAction = "fs:write"
	FSCreate  FilesystemAction = "fs:create"
	FSExecute FilesystemAction = "fs:execute"
)

var validFilesystemActions = map[FilesystemAction]struct{}{
	FSRead: {}, FSList: {}, FSWrite: {}, FSCreate: {}, FSExecute: {},
}

// FilesystemRule is one entry in an agent's filesystem override block.
// Paths are stored resolved to absolute after workspace loading.
type FilesystemRule struct {
	Action  []FilesystemAction `yaml:"action"            json:"action"`
	Path    string             `yaml:"path"              json:"path"`
	Exclude []string           `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

// AgentEntry is the validated, resolved form of a single agent declaration
// in workspace.yaml. This is the complete per-agent config surface.
// There are no per-agent config files and no base merge.
type AgentEntry struct {
	Name       string           `yaml:"name"                 json:"name"`
	Model      string           `yaml:"model,omitempty"      json:"model,omitempty"`
	Filesystem []FilesystemRule `yaml:"filesystem,omitempty" json:"filesystem,omitempty"`

	// ResolvedModel is populated during cross-file validation; not stored in YAML.
	ResolvedModel *model.ResolvedModelRef `yaml:"-" json:"-"`
}

var agentNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// AgentRegistry is a lookup structure over the validated agent entries
// declared in workspace.yaml.
type AgentRegistry struct {
	entries map[string]*AgentEntry
	order   []string
}

// Get returns the agent entry for the given name.
func (r *AgentRegistry) Get(name string) (*AgentEntry, bool) {
	if r == nil {
		return nil, false
	}
	e, ok := r.entries[name]
	return e, ok
}

// Names returns all agent names in sorted order.
func (r *AgentRegistry) Names() []string {
	if r == nil || len(r.entries) == 0 {
		return nil
	}
	names := make([]string, 0, len(r.entries))
	for name := range r.entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// All returns all agent entries in declaration order.
func (r *AgentRegistry) All() []*AgentEntry {
	if r == nil || len(r.entries) == 0 {
		return nil
	}
	result := make([]*AgentEntry, 0, len(r.entries))
	for _, name := range r.order {
		result = append(result, r.entries[name])
	}
	return result
}

// buildAgentRegistry constructs an AgentRegistry from a validated slice.
// The entries slice must already be validated (no duplicate names).
func buildAgentRegistry(entries []AgentEntry) (*AgentRegistry, error) {
	reg := &AgentRegistry{
		entries: make(map[string]*AgentEntry, len(entries)),
		order:   make([]string, 0, len(entries)),
	}
	for i := range entries {
		e := &entries[i]
		reg.entries[e.Name] = e
		reg.order = append(reg.order, e.Name)
	}
	return reg, nil
}

// resolveAgentPaths replaces ${workspace} with workspaceAbs in all agent
// filesystem paths and excludes. This must run before validateAgents.
func resolveAgentPaths(agents []AgentEntry, workspaceAbs string) {
	for i := range agents {
		for j := range agents[i].Filesystem {
			agents[i].Filesystem[j].Path = strings.ReplaceAll(
				agents[i].Filesystem[j].Path, "${workspace}", workspaceAbs)
			for k := range agents[i].Filesystem[j].Exclude {
				agents[i].Filesystem[j].Exclude[k] = strings.ReplaceAll(
					agents[i].Filesystem[j].Exclude[k], "${workspace}", workspaceAbs)
			}
		}
	}
}

// injectConfigProtection unconditionally appends the relurpify_cfg and .git
// exclude paths to every filesystem rule that grants write or execute access.
// This runs after resolveAgentPaths so paths are absolute.
func injectConfigProtection(agents []AgentEntry, workspaceAbs string) {
	cfgPath := filepath.Join(workspaceAbs, "relurpify_cfg") + "/**"
	gitPath := filepath.Join(workspaceAbs, ".git") + "/**"
	for i := range agents {
		for j := range agents[i].Filesystem {
			if ruleGrantsWriteOrExecute(agents[i].Filesystem[j]) {
				agents[i].Filesystem[j].Exclude = appendExcludeIfAbsent(agents[i].Filesystem[j].Exclude, cfgPath)
				agents[i].Filesystem[j].Exclude = appendExcludeIfAbsent(agents[i].Filesystem[j].Exclude, gitPath)
			}
		}
	}
}

func ruleGrantsWriteOrExecute(rule FilesystemRule) bool {
	for _, a := range rule.Action {
		if a == FSWrite || a == FSCreate || a == FSExecute {
			return true
		}
	}
	return false
}

func appendExcludeIfAbsent(slice []string, value string) []string {
	for _, s := range slice {
		if s == value {
			return slice
		}
	}
	return append(slice, value)
}

// resolveAgentModels resolves the model name override for each agent entry
// that declares one, setting ResolvedModel. The provider always comes from
// the workspace — agents override the model name only.
func resolveAgentModels(agents []AgentEntry, workspaceModel model.ModelRef, providers []*model.ResolvedProvider) error {
	var errs []string
	for i := range agents {
		entry := &agents[i]
		ref := model.ModelRef{
			Provider: workspaceModel.Provider,
			Name:     workspaceModel.Name,
		}
		if entry.Model != "" {
			ref.Name = entry.Model
		}
		resolved, err := model.ResolveModelRef(ref, workspaceModel, providers)
		if err != nil {
			errs = append(errs, fmt.Sprintf("agents[name=%s].model: %v", entry.Name, err))
			continue
		}
		entry.ResolvedModel = resolved
	}
	if len(errs) > 0 {
		return fmt.Errorf("agent model resolution:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}
