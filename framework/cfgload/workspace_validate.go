package cfgload

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/cfgload/model"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
)

// validateAgents validates the agent entries parsed from workspace.yaml.
// Paths must already be resolved to absolute by resolveAgentPaths before this is called.
func validateAgents(agents []AgentEntry, workspaceAbs string) error {
	if len(agents) == 0 {
		return nil
	}
	var errs []error
	seen := make(map[string]int, len(agents))
	for i, entry := range agents {
		prefix := fmt.Sprintf("agents[%d]", i)

		if strings.TrimSpace(entry.Name) == "" {
			errs = append(errs, fmt.Errorf("%s.name: required", prefix))
		} else if !agentNameRegex.MatchString(entry.Name) {
			errs = append(errs, fmt.Errorf("%s.name: %q must match ^[a-zA-Z][a-zA-Z0-9_-]*$", prefix, entry.Name))
		} else if prev, dup := seen[entry.Name]; dup {
			errs = append(errs, fmt.Errorf("%s.name: %q duplicates agents[%d].name", prefix, entry.Name, prev))
		} else {
			seen[entry.Name] = i
		}

		if entry.Model != "" && strings.TrimSpace(entry.Model) == "" {
			errs = append(errs, fmt.Errorf("%s.model: must not be blank if present", prefix))
		}

		if entry.Filesystem != nil && len(entry.Filesystem) == 0 {
			errs = append(errs, fmt.Errorf("%s.filesystem: must contain at least one rule if present", prefix))
		}

		for j, rule := range entry.Filesystem {
			rulePrefix := fmt.Sprintf("%s.filesystem[%d]", prefix, j)

			if len(rule.Action) == 0 {
				errs = append(errs, fmt.Errorf("%s.action: required", rulePrefix))
			}
			for k, action := range rule.Action {
				if _, ok := validFilesystemActions[action]; !ok {
					errs = append(errs, fmt.Errorf("%s.action[%d]: unknown value %q; valid: fs:read, fs:list, fs:write, fs:create, fs:execute", rulePrefix, k, action))
				}
			}

			if rule.Path == "" {
				errs = append(errs, fmt.Errorf("%s.path: required", rulePrefix))
			} else if !strings.HasPrefix(rule.Path, workspaceAbs) {
				errs = append(errs, fmt.Errorf("%s.path: must begin with ${workspace}", rulePrefix))
			} else if containsPathTraversal(rule.Path) {
				errs = append(errs, fmt.Errorf("%s.path: must not contain path traversal", rulePrefix))
			}

			for k, ex := range rule.Exclude {
				if !strings.HasPrefix(ex, workspaceAbs) {
					errs = append(errs, fmt.Errorf("%s.exclude[%d]: must begin with ${workspace}", rulePrefix, k))
				} else if containsPathTraversal(ex) {
					errs = append(errs, fmt.Errorf("%s.exclude[%d]: must not contain path traversal", rulePrefix, k))
				}
			}
		}
	}
	return errors.Join(errs...)
}

func containsPathTraversal(path string) bool {
	cleaned := filepath.Clean(strings.TrimSuffix(path, "/**"))
	return strings.Contains(cleaned, ".."+string(filepath.Separator)) || strings.HasSuffix(cleaned, "..")
}

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
	if backend != "" && !sandbox.IsSupportedSandboxBackend(backend) {
		supported := strings.Join(sandbox.SupportedSandboxBackends(), ", ")
		errs = append(errs, fmt.Errorf("sandbox.backend must be one of %s (got %q)", supported, backend))
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

	if agentErr := validateAgents(c.Agents, c.WorkspaceAbs); agentErr != nil {
		errs = append(errs, agentErr)
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
