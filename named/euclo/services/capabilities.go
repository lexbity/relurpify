package services

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
)

// defaultCapabilityRegistrar implements CapabilityRegistrar using Euclo's relurpic capabilities.
type defaultCapabilityRegistrar struct{}

func (r *defaultCapabilityRegistrar) RegisterAll(env agentenv.WorkspaceEnvironment) error {
	if env.Registry == nil {
		return fmt.Errorf("capability registry is nil")
	}
	if env.Config == nil || env.Config.AgentSpec == nil {
		return fmt.Errorf("agent spec required for relurpic capability registration")
	}
	declared := declaredRelurpicCapabilities(env.Config.AgentSpec.Capabilities.Relurpic)
	if len(declared) == 0 {
		return fmt.Errorf("capabilities.relurpic required")
	}
	return relurpicabilities.RegisterAll(env, declared)
}

func declaredRelurpicCapabilities(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))
	for _, id := range input {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
