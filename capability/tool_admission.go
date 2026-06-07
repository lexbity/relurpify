package capability

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// ToolAdmissionPolicy gates local tool registration against declared tool
// manifests loaded from relurpify_cfg/tools.
type ToolAdmissionPolicy struct {
	manifests map[string]ports.ToolManifest
}

// NewToolAdmissionPolicy builds an admission policy from manifest records.
func NewToolAdmissionPolicy(manifests []*ports.ToolManifest) *ToolAdmissionPolicy {
	if len(manifests) == 0 {
		return &ToolAdmissionPolicy{manifests: map[string]ports.ToolManifest{}}
	}
	byName := make(map[string]ports.ToolManifest, len(manifests))
	for _, manifest := range manifests {
		if manifest == nil {
			continue
		}
		name := ports.NormalizeToolName(manifest.Name)
		if name == "" {
			continue
		}
		byName[name] = *manifest
	}
	return &ToolAdmissionPolicy{manifests: byName}
}

// Manifest resolves a declared manifest by normalized tool name.
func (p *ToolAdmissionPolicy) Manifest(name string) (ports.ToolManifest, bool) {
	if p == nil {
		return ports.ToolManifest{}, false
	}
	manifest, ok := p.manifests[ports.NormalizeToolName(name)]
	return manifest, ok
}

// DeclaredNames returns the normalized names that were admitted from config.
func (p *ToolAdmissionPolicy) DeclaredNames() []string {
	if p == nil {
		return nil
	}
	names := make([]string, 0, len(p.manifests))
	for name := range p.manifests {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Admit evaluates a tool against the loaded manifests.
func (p *ToolAdmissionPolicy) Admit(tool ports.Tool) (bool, error) {
	if tool == nil {
		return false, fmt.Errorf("tool required")
	}
	if p == nil {
		return true, nil
	}
	name := ports.NormalizeToolName(tool.Name())
	manifest, ok := p.manifests[name]
	if !ok {
		log.Printf("tool admission warning: skipping unlisted tool %q", tool.Name())
		return false, nil
	}
	if manifest.Execution.Backend == ports.ToolBackendGoNative {
		if strings.TrimSpace(manifest.Execution.Implementation) != "" &&
			ports.NormalizeToolName(manifest.Execution.Implementation) != name {
			log.Printf("tool admission warning: skipping %q due to implementation mismatch: %q", tool.Name(), manifest.Execution.Implementation)
			return false, nil
		}
	}
	if strings.TrimSpace(manifest.Description) != "" && strings.TrimSpace(tool.Description()) == "" {
		return false, fmt.Errorf("tool %q missing description", tool.Name())
	}
	return true, nil
}
