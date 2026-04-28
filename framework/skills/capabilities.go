package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/manifest"
)

type capabilityRegistrar interface {
	RegisterCapability(descriptor core.CapabilityDescriptor) error
	RegisterPromptCapability(handler core.PromptCapabilityHandler) error
	RegisterResourceCapability(handler core.ResourceCapabilityHandler) error
}

func registerSkillCapabilities(registry capabilityRegistrar, skill *manifest.SkillManifest, paths SkillPaths) error {
	if registry == nil || skill == nil {
		return nil
	}
	candidates := EnumerateSkillCapabilities([]ResolvedSkill{{Manifest: skill, Paths: paths}})
	if batchRegistry, ok := registry.(interface {
		RegisterBatch([]capability.RegistrationBatchItem) error
	}); ok {
		items := make([]capability.RegistrationBatchItem, 0, len(candidates))
		for _, candidate := range candidates {
			item := capability.RegistrationBatchItem{Descriptor: candidate.Descriptor}
			if candidate.PromptHandler != nil {
				item.PromptHandler = candidate.PromptHandler
			}
			if candidate.ResourceHandler != nil {
				item.ResourceHandler = candidate.ResourceHandler
			}
			items = append(items, item)
		}
		return batchRegistry.RegisterBatch(items)
	}
	for _, candidate := range candidates {
		switch {
		case candidate.PromptHandler != nil:
			if err := registry.RegisterPromptCapability(candidate.PromptHandler); err != nil {
				return err
			}
		case candidate.ResourceHandler != nil:
			if err := registry.RegisterResourceCapability(candidate.ResourceHandler); err != nil {
				return err
			}
		case candidate.Descriptor.ID != "":
			if err := registry.RegisterCapability(candidate.Descriptor); err != nil {
				return err
			}
		default:
			continue
		}
	}
	return nil
}

// EnumerateSkillCapabilities expands resolved skills into prompt/resource
// capability candidates without mutating any registry state.
func EnumerateSkillCapabilities(resolved []ResolvedSkill) []SkillCapabilityCandidate {
	out := make([]SkillCapabilityCandidate, 0)
	for _, entry := range resolved {
		if entry.Manifest == nil {
			continue
		}
		for _, descriptor := range skillCapabilityDescriptors(entry.Manifest, entry.Paths) {
			out = append(out, SkillCapabilityCandidate{Descriptor: descriptor})
		}
		for _, prompt := range skillPromptCapabilities(entry.Manifest) {
			out = append(out, SkillCapabilityCandidate{
				Descriptor:    prompt.Descriptor(context.Background(), nil),
				PromptHandler: prompt,
			})
		}
		for _, resource := range skillResourceCapabilities(entry.Manifest, entry.Paths) {
			out = append(out, SkillCapabilityCandidate{
				Descriptor:      resource.Descriptor(context.Background(), nil),
				ResourceHandler: resource,
			})
		}
	}
	return out
}

func skillCapabilityDescriptors(skill *manifest.SkillManifest, paths SkillPaths) []core.CapabilityDescriptor {
	if skill == nil {
		return nil
	}
	var descriptors []core.CapabilityDescriptor
	return descriptors
}

func skillPromptCapabilities(skill *manifest.SkillManifest) []skillPromptCapability {
	if skill == nil {
		return nil
	}
	out := make([]skillPromptCapability, 0, len(skill.Spec.PromptSnippets))
	for i, snippet := range skill.Spec.PromptSnippets {
		snippet = strings.TrimSpace(snippet)
		if snippet == "" {
			continue
		}
		out = append(out, skillPromptCapability{
			skillName: skill.Metadata.Name,
			version:   skill.Metadata.Version,
			index:     i + 1,
			snippet:   snippet,
		})
	}
	return out
}

func skillResourceCapabilities(skill *manifest.SkillManifest, paths SkillPaths) []*skillResourceCapability {
	if skill == nil {
		return nil
	}
	out := make([]*skillResourceCapability, 0, len(paths.Resources))
	for _, resourcePath := range paths.Resources {
		clean := strings.TrimSpace(resourcePath)
		if clean == "" {
			continue
		}
		out = append(out, &skillResourceCapability{
			skillName: skill.Metadata.Name,
			version:   skill.Metadata.Version,
			path:      clean,
			base:      filepath.Base(clean),
		})
	}
	return out
}

type skillPromptCapability struct {
	skillName string
	version   string
	index     int
	snippet   string
}

func (c skillPromptCapability) Descriptor(context.Context, *contextdata.Envelope) core.CapabilityDescriptor {
	name := fmt.Sprintf("%s.prompt.%d", c.skillName, c.index)
	return core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
		ID:          fmt.Sprintf("prompt:%s:%d", c.skillName, c.index),
		Kind:        core.CapabilityKindPrompt,
		Name:        name,
		Version:     c.version,
		Description: truncateSkillCapabilityDescription(c.snippet),
		Category:    "skill-prompt",
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeWorkspace,
		},
		TrustClass: core.TrustClassWorkspaceTrusted,
		RiskClasses: []core.RiskClass{
			core.RiskClassReadOnly,
		},
		EffectClasses: []core.EffectClass{
			core.EffectClassContextInsertion,
		},
		Availability: core.AvailabilitySpec{Available: true},
		Annotations: map[string]any{
			"skill":       c.skillName,
			"index":       c.index,
			"prompt_text": c.snippet,
		},
	})
}

func (c skillPromptCapability) RenderPrompt(_ context.Context, _ *contextdata.Envelope, args map[string]interface{}) (*core.PromptRenderResult, error) {
	content := c.snippet
	for key, value := range args {
		content = strings.ReplaceAll(content, "{"+key+"}", fmt.Sprint(value))
	}
	return &core.PromptRenderResult{
		Description: truncateSkillCapabilityDescription(c.snippet),
		Messages: []core.PromptMessage{{
			Content: []core.ContentBlock{core.TextContentBlock{Text: content}},
		}},
	}, nil
}

type skillResourceCapability struct {
	skillName string
	version   string
	path      string
	base      string
	manager   *authorization.PermissionManager
	agentID   string
}

func (c skillResourceCapability) Descriptor(context.Context, *contextdata.Envelope) core.CapabilityDescriptor {
	return core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
		ID:          fmt.Sprintf("resource:%s:%s", c.skillName, filepath.ToSlash(c.base)),
		Kind:        core.CapabilityKindResource,
		Name:        fmt.Sprintf("%s.resource.%s", c.skillName, c.base),
		Version:     c.version,
		Description: fmt.Sprintf("Skill resource %s", c.base),
		Category:    "skill-resource",
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeWorkspace,
		},
		TrustClass: core.TrustClassWorkspaceTrusted,
		RiskClasses: []core.RiskClass{
			core.RiskClassReadOnly,
		},
		Availability: core.AvailabilitySpec{Available: true},
		Annotations: map[string]any{
			"skill":     c.skillName,
			"path":      c.path,
			"kind":      "resource-file",
			"mime_type": inferSkillResourceMIMEType(c.base),
		},
	})
}

func (c skillResourceCapability) ReadResource(ctx context.Context, _ *contextdata.Envelope) (*core.ResourceReadResult, error) {
	if c.manager != nil {
		if err := c.manager.CheckFileAccess(ctx, c.agentID, core.FileSystemRead, c.path); err != nil {
			return nil, err
		}
	}
	data, err := os.ReadFile(c.path)
	if err != nil {
		return nil, err
	}
	return &core.ResourceReadResult{
		Contents: []core.ContentBlock{core.TextContentBlock{Text: string(data)}},
		Metadata: map[string]any{
			"path": c.path,
		},
	}, nil
}

func inferSkillResourceMIMEType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md":
		return "text/markdown"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".json":
		return "application/json"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

func truncateSkillCapabilityDescription(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= 96 {
		return text
	}
	return strings.TrimSpace(text[:93]) + "..."
}
