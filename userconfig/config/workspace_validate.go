package config

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/userconfig/config/model"
)

var supportedSandboxBackends = map[string]struct{}{
	"docker": {},
	"gvisor": {},
}

// Validate enforces the workspace root contract and field constraints.
func (c *WorkspaceConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("workspace config required")
	}
	backend := strings.ToLower(strings.TrimSpace(stringValue(c.Sandbox.Backend)))
	if backend != "" {
		if _, ok := supportedSandboxBackends[backend]; !ok {
			supported := "docker, gvisor"
			return fmt.Errorf("sandbox.backend must be one of %s (got %q)", supported, backend)
		}
	}
	return nil
}

// ValidateModelRef resolves the workspace model against the provider registry.
func (c *WorkspaceConfig) ValidateModelRef(providers []*model.ResolvedProvider) error {
	if c == nil {
		return fmt.Errorf("workspace config required")
	}
	if _, err := model.ResolveModelRef(c.Model, model.ModelRef{}, providers); err != nil {
		return fmt.Errorf("workspace model: %w", err)
	}
	return nil
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
