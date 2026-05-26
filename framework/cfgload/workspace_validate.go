package cfgload

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/cfgload/model"
)

// Validate enforces the workspace root contract and field constraints.
func (c *WorkspaceConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("workspace config required")
	}
	var errs []error

	if strings.TrimSpace(c.WorkspaceAbs) == "" {
		errs = append(errs, fmt.Errorf("workspace root required"))
	} else if !filepath.IsAbs(c.WorkspaceAbs) {
		errs = append(errs, fmt.Errorf("workspace root must be absolute: %q", c.WorkspaceAbs))
	}

	stateDir := strings.TrimSpace(c.stateDirValue())
	if stateDir == "" {
		errs = append(errs, fmt.Errorf("paths.state_dir required"))
	} else {
		if filepath.IsAbs(stateDir) {
			errs = append(errs, fmt.Errorf("paths.state_dir must be relative: %q", stateDir))
		}
		clean := filepath.Clean(stateDir)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			errs = append(errs, fmt.Errorf("paths.state_dir must stay within the workspace: %q", stateDir))
		}
	}

	backend := strings.ToLower(strings.TrimSpace(stringValue(c.Sandbox.Backend)))
	switch backend {
	case "gvisor", "docker", "local":
	default:
		errs = append(errs, fmt.Errorf("sandbox.backend must be one of gvisor, docker, local"))
	}

	level := strings.ToLower(strings.TrimSpace(stringValue(c.Logging.Level)))
	switch level {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("logging.level must be one of debug, info, warn, error"))
	}

	format := strings.ToLower(strings.TrimSpace(stringValue(c.Logging.Format)))
	switch format {
	case "json", "text":
	default:
		errs = append(errs, fmt.Errorf("logging.format must be one of json, text"))
	}

	if c.Audit.RetentionDays == nil {
		errs = append(errs, fmt.Errorf("audit.retention_days required"))
	} else if *c.Audit.RetentionDays < 1 || *c.Audit.RetentionDays > 365 {
		errs = append(errs, fmt.Errorf("audit.retention_days must be between 1 and 365"))
	}

	if len(errs) > 0 {
		return fmt.Errorf("workspace validation failed: %w", errors.Join(errs...))
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
