package config

import (
	"errors"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

func validateToolManifest(path string, manifest *ports.ToolManifest) error {
	if manifest == nil {
		return &SchemaError{Path: path, Err: fmt.Errorf("tool manifest required")}
	}
	var problems []string
	if ports.NormalizeToolName(manifest.Name) == "" {
		problems = append(problems, "name required")
	}
	if ports.NormalizeToolName(manifest.Family) == "" {
		problems = append(problems, "family required")
	}
	for i, intent := range manifest.Intent {
		if ports.NormalizeToolName(intent) == "" {
			problems = append(problems, fmt.Sprintf("intent[%d] required", i))
		}
	}
	if strings.TrimSpace(manifest.Description) == "" {
		problems = append(problems, "description required")
	}
	if err := validateToolManifestParameters(manifest.Parameters); err != nil {
		problems = append(problems, err.Error())
	}
	if err := validateToolManifestExecution(manifest.Execution); err != nil {
		problems = append(problems, err.Error())
	}
	if err := validateToolManifestCapability(manifest.Capability); err != nil {
		problems = append(problems, err.Error())
	}
	if err := validateToolManifestFlags(manifest.Execution.Command, manifest.Parameters); err != nil {
		problems = append(problems, err.Error())
	}
	if len(problems) > 0 {
		return &SchemaError{
			Path: path,
			Err:  errors.New(strings.Join(problems, "; ")),
		}
	}
	return nil
}

func validateToolManifestParameters(params []ports.ToolParameter) error {
	seen := make(map[string]struct{}, len(params))
	var problems []string
	for i, param := range params {
		name := ports.NormalizeToolName(param.Name)
		if name == "" {
			problems = append(problems, fmt.Sprintf("parameters[%d].name required", i))
			continue
		}
		if _, ok := seen[name]; ok {
			problems = append(problems, fmt.Sprintf("parameters[%d].name duplicate", i))
			continue
		}
		seen[name] = struct{}{}
		if strings.TrimSpace(string(param.Type)) == "" {
			problems = append(problems, fmt.Sprintf("parameters[%d].type required", i))
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func validateToolManifestExecution(exec ports.ToolManifestExecution) error {
	switch exec.Backend {
	case ports.ToolBackendSubprocess:
		if exec.Command == nil || len(exec.Command.Base) == 0 {
			return fmt.Errorf("execution.command.base required for subprocess backend")
		}
	case ports.ToolBackendGoNative:
		if strings.TrimSpace(exec.Implementation) == "" {
			return fmt.Errorf("execution.implementation required for go_native backend")
		}
	case ports.ToolBackendComposite:
		// composition.steps validated during build, not here
	default:
		return fmt.Errorf("execution.backend unsupported")
	}
	if exec.Sandbox != nil && exec.Sandbox.TimeoutSeconds < 0 {
		return fmt.Errorf("execution.sandbox.timeout_seconds must be non-negative")
	}
	return nil
}

func validateToolManifestCapability(cap ports.ToolManifestCapability) error {
	if strings.TrimSpace(cap.TrustClass) == "" {
		return fmt.Errorf("capability.trust_class required")
	}
	if len(cap.RiskClass) == 0 {
		return fmt.Errorf("capability.risk_class required")
	}
	if len(cap.EffectClass) == 0 {
		return fmt.Errorf("capability.effect_class required")
	}
	return nil
}

// validateToolManifestFlags checks each declared flag for internal consistency.
// A flag must use exactly one form (boolean WhenTrue/WhenFalse or typed Param),
// typed flags must reference a declared parameter, and Style must be valid.
func validateToolManifestFlags(cmd *ports.ToolManifestCommand, params []ports.ToolParameter) error {
	if cmd == nil || len(cmd.Flags) == 0 {
		return nil
	}

	paramNames := make(map[string]struct{}, len(params))
	for _, p := range params {
		paramNames[ports.NormalizeToolName(p.Name)] = struct{}{}
	}

	validStyles := map[string]bool{
		ports.FlagStyleEquals:   true,
		ports.FlagStyleSeparate: true,
	}

	var problems []string
	for name, flag := range cmd.Flags {
		hasBool := len(flag.WhenTrue) > 0 || len(flag.WhenFalse) > 0
		hasTyped := flag.Param != ""

		if hasBool && hasTyped {
			problems = append(problems, fmt.Sprintf("flag %q: must use exactly one of boolean (when_true/when_false) or typed (param) form", name))
			continue
		}
		if !hasBool && !hasTyped {
			problems = append(problems, fmt.Sprintf("flag %q: must specify either boolean (when_true/when_false) or typed (param) form", name))
			continue
		}

		if hasTyped {
			if _, ok := paramNames[ports.NormalizeToolName(flag.Param)]; !ok {
				problems = append(problems, fmt.Sprintf("flag %q: param %q does not match any declared parameter", name, flag.Param))
			}
			if flag.Style != "" && !validStyles[flag.Style] {
				problems = append(problems, fmt.Sprintf("flag %q: style %q must be %q or %q", name, flag.Style, ports.FlagStyleEquals, ports.FlagStyleSeparate))
			}
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}
