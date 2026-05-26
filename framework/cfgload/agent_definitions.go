package cfgload

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"gopkg.in/yaml.v3"
)

// LoadAgentDefinitions loads standalone agent definition files from dir.
func LoadAgentDefinitions(workspace, dir string) (map[string]*agentspec.AgentDefinition, error) {
	defs := make(map[string]*agentspec.AgentDefinition)
	if strings.TrimSpace(dir) == "" {
		return defs, nil
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve agent definitions dir: %w", err)
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defs, nil
		}
		return nil, fmt.Errorf("read agent definitions dir: %w", err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		paths = append(paths, filepath.Join(absDir, name))
	}
	sort.Strings(paths)

	for _, path := range paths {
		body, err := ReadConfigFile(workspace, path)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", filepath.Base(path), err)
		}

		var header struct {
			Kind string `yaml:"kind"`
		}
		if err := yaml.Unmarshal(body, &header); err != nil {
			return nil, fmt.Errorf("load %s: %w", filepath.Base(path), err)
		}
		if header.Kind != "" && !strings.EqualFold(header.Kind, "AgentDefinition") {
			continue
		}
		if err := RejectForbiddenSecretFields(path, body); err != nil {
			return nil, err
		}

		var def agentspec.AgentDefinition
		if err := yaml.Unmarshal(body, &def); err != nil {
			return nil, fmt.Errorf("load %s: %w", filepath.Base(path), err)
		}
		if err := def.Spec.Validate(); err != nil {
			return nil, fmt.Errorf("agent spec invalid: %w", err)
		}
		if def.Name == "" {
			base := filepath.Base(path)
			base = strings.TrimSuffix(base, ".agent.yaml")
			base = strings.TrimSuffix(base, ".yaml")
			base = strings.TrimSuffix(base, ".yml")
			def.Name = base
		}
		defs[def.Name] = &def
	}

	return defs, nil
}
