package cfgload

import (
	"errors"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

func validateToolManifest(path string, manifest *contracts.ToolManifest) error {
	if manifest == nil {
		return &SchemaError{Path: path, Err: fmt.Errorf("tool manifest required")}
	}
	var problems []string
	if contracts.NormalizeToolName(manifest.Name) == "" {
		problems = append(problems, "name required")
	}
	if contracts.NormalizeToolName(manifest.Family) == "" {
		problems = append(problems, "family required")
	}
	for i, intent := range manifest.Intent {
		if contracts.NormalizeToolName(intent) == "" {
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
	if len(problems) > 0 {
		return &SchemaError{
			Path: path,
			Err:  errors.New(strings.Join(problems, "; ")),
		}
	}
	return nil
}

func validateToolManifestParameters(params []contracts.ToolParameter) error {
	seen := make(map[string]struct{}, len(params))
	var problems []string
	for i, param := range params {
		name := contracts.NormalizeToolName(param.Name)
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

func validateToolManifestExecution(exec contracts.ToolManifestExecution) error {
	switch exec.Backend {
	case contracts.ToolBackendSubprocess:
		if exec.Command == nil || len(exec.Command.Base) == 0 {
			return fmt.Errorf("execution.command.base required for subprocess backend")
		}
	case contracts.ToolBackendGoNative:
		if strings.TrimSpace(exec.Implementation) == "" {
			return fmt.Errorf("execution.implementation required for go_native backend")
		}
	case contracts.ToolBackendMCP:
		if exec.MCP == nil {
			return fmt.Errorf("execution.mcp required for mcp backend")
		}
		if strings.TrimSpace(exec.MCP.Server) == "" || strings.TrimSpace(exec.MCP.Method) == "" {
			return fmt.Errorf("execution.mcp.server and execution.mcp.method required")
		}
	default:
		return fmt.Errorf("execution.backend unsupported")
	}
	if exec.Sandbox != nil && exec.Sandbox.TimeoutSeconds < 0 {
		return fmt.Errorf("execution.sandbox.timeout_seconds must be non-negative")
	}
	return nil
}

func validateToolManifestCapability(cap contracts.ToolManifestCapability) error {
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
