package cfgload

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/cfgload/model"
	"gopkg.in/yaml.v3"
)

// LoadAgentRegistry loads _base.agent.yaml and all named agent files, merges
// them, resolves model references, and validates capability tool references.
func LoadAgentRegistry(dir string, workspace string, env []string, workspaceDefault model.ModelRef, providers []*model.ResolvedProvider, toolRegistry *ToolRegistry, decoder func(string, []byte, any) (any, error)) (*AgentRegistry, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve agents dir: %w", err)
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, fmt.Errorf("read agents dir: %w", err)
	}

	var errs []error
	registry := &AgentRegistry{Agents: make(map[string]*AgentConfig)}

	basePath := filepath.Join(absDir, "_base.agent.yaml")
	baseAgent, err := loadAgentFile(basePath, workspace, env, workspaceDefault, decoder)
	if err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", basePath, err))
		baseAgent = &AgentConfig{}
	} else if strings.ToLower(strings.TrimSpace(baseAgent.Kind)) != "base" {
		errs = append(errs, fmt.Errorf("%s: kind must be base", basePath))
	}

	namedPaths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "_base.agent.yaml" || (!strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml")) {
			continue
		}
		namedPaths = append(namedPaths, filepath.Join(absDir, name))
	}
	sort.Strings(namedPaths)

	seenNames := make(map[string]string, len(namedPaths))
	for _, path := range namedPaths {
		namedAgent, err := loadAgentFile(path, workspace, env, workspaceDefault, decoder)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		if strings.ToLower(strings.TrimSpace(namedAgent.Kind)) != "agent" {
			errs = append(errs, fmt.Errorf("%s: kind must be agent", path))
			continue
		}
		if strings.TrimSpace(namedAgent.Name) == "" {
			namedAgent.Name = agentNameFromPath(path)
		}

		merged := MergeAgentConfig(baseAgent, namedAgent)
		merged.SourcePath = path
		applyFilesystemSecurityInvariant(merged, workspace)

		if prev, ok := seenNames[strings.ToLower(strings.TrimSpace(merged.Name))]; ok {
			errs = append(errs, fmt.Errorf("duplicate agent name %q in %s and %s", merged.Name, prev, path))
			continue
		}
		seenNames[strings.ToLower(strings.TrimSpace(merged.Name))] = path

		resolvedModel, err := model.ResolveModelRef(merged.Model, workspaceDefault, providers)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s (%s) model: %w", merged.Name, path, err))
		} else {
			merged.ResolvedModel = resolvedModel
		}

		if toolRegistry == nil {
			errs = append(errs, fmt.Errorf("%s (%s): tool registry required", merged.Name, path))
		} else if err := validateAgentTools(merged, toolRegistry); err != nil {
			errs = append(errs, fmt.Errorf("%s (%s): %w", merged.Name, path, err))
		}

		registry.Agents[merged.Name] = merged
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return registry, nil
}

func loadAgentFile(path, workspace string, env []string, defaultModel model.ModelRef, decoder func(string, []byte, any) (any, error)) (*AgentConfig, error) {
	body, err := ReadConfigFile(workspace, path)
	if err != nil {
		return nil, err
	}

	decl, rawBody, err := SplitSchemaDocument(path, body)
	if err != nil {
		return nil, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(rawBody, &doc); err != nil {
		return nil, err
	}
	if err := resolveNodeVariables(&doc, workspace, env, defaultModel); err != nil {
		return nil, err
	}
	resolvedBody, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, err
	}
	resolvedData := append([]byte("schema: "+decl.String()+"\n"), resolvedBody...)

	var agent AgentConfig
	if decoder != nil {
		if err := RejectForbiddenSecretFields(path, resolvedBody); err != nil {
			return nil, err
		}
		if err := NewSchemaRegistry().Require(decl); err != nil {
			return nil, err
		}
		if err := rejectAnchors(&doc, path, decl.Line); err != nil {
			return nil, err
		}
		if _, err := decoder(path, resolvedData, &agent); err != nil {
			return nil, err
		}
	} else {
		if _, err := DecodeWithSchema(path, resolvedData, NewSchemaRegistry(), &agent); err != nil {
			return nil, err
		}
	}

	agent.SourcePath = path
	return &agent, nil
}

func validateAgentTools(agent *AgentConfig, toolRegistry *ToolRegistry) error {
	if agent == nil {
		return fmt.Errorf("agent required")
	}
	if toolRegistry == nil {
		return fmt.Errorf("tool registry required")
	}
	var missing []string
	for _, toolName := range agent.Capabilities.Tools {
		toolName = strings.TrimSpace(toolName)
		if toolName == "" {
			continue
		}
		if _, ok := toolRegistry.LookupTool(toolName); !ok {
			missing = append(missing, toolName)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("unknown capability tool(s): %s", strings.Join(missing, ", "))
	}
	return nil
}

func agentNameFromPath(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".agent.yaml")
	base = strings.TrimSuffix(base, ".yaml")
	base = strings.TrimSuffix(base, ".yml")
	return base
}
