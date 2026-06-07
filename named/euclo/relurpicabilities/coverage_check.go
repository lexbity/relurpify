package relurpicabilities

import (
	"context"
	"fmt"
	"time"

	capability "codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/capability/schemacoerce"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution/agentenv"
)

// CoverageCheckHandler implements the test coverage capability.
type CoverageCheckHandler struct {
	env agentenv.WorkspaceEnvironment
	frameworkPolicyContext
}

// NewCoverageCheckHandler creates a new coverage check handler.
func NewCoverageCheckHandler(env agentenv.WorkspaceEnvironment) *CoverageCheckHandler {
	return &CoverageCheckHandler{env: env}
}

// Descriptor returns the capability descriptor for the coverage check handler.
func (h *CoverageCheckHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) capability.CapabilityDescriptor {
	return capability.CapabilityDescriptor{
		ID:            "euclo:cap.coverage_check",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Name:          "Coverage Check",
		Version:       "1.0.0",
		Description:   "Runs test coverage analysis and reports per-package coverage percentages",
		Category:      "verification",
		Tags:          []string{"testing", "coverage", "shell", "tool"},
		Source: capability.CapabilitySource{
			Scope: agentspec.CapabilityScopeBuiltin,
		},
		TrustClass:    agentspec.TrustClassBuiltinTrusted,
		RiskClasses:   []agentspec.RiskClass{agentspec.RiskClassExecute},
		EffectClasses: []agentspec.EffectClass{agentspec.EffectClassProcessSpawn},
		InputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"package": {
					Type:        "string",
					Description: "Go package path to check (default: ./...)",
				},
				"threshold": {
					Type:        "number",
					Description: "Fail if coverage is below this percentage (0-100)",
				},
			},
		},
		OutputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"success": {
					Type:        "boolean",
					Description: "True if coverage run completed",
				},
				"passed": {
					Type:        "boolean",
					Description: "True if coverage meets threshold",
				},
				"packages": {
					Type:        "array",
					Description: "Per-package coverage results",
					Items:       &schemacoerce.Schema{Type: "object"},
				},
				"coverage": {
					Type:        "object",
					Description: "Per-package coverage percentages",
				},
				"total_coverage": {
					Type:        "number",
					Description: "Average coverage across all packages",
				},
				"threshold": {
					Type:        "number",
					Description: "Coverage threshold used for pass/fail evaluation",
				},
				"exit_code": {
					Type:        "integer",
					Description: "Exit code from the coverage command",
				},
				"package": {
					Type:        "string",
					Description: "Resolved package path passed to go test",
				},
				"output": {
					Type:        "string",
					Description: "Raw coverage output",
				},
			},
		},
	}
}

// Invoke runs go test -cover and returns per-package coverage results.
func (h *CoverageCheckHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*ports.CapabilityExecutionResult, error) {
	if h.env.CommandRunner == nil {
		return failResult("CommandRunner not available in environment"), fmt.Errorf("command runner not available")
	}

	pkg, _ := stringArg(args, "package")
	if pkg == "" {
		pkg = "./..."
	}
	threshold, _ := floatArg(args, "threshold", 0)
	if threshold < 0 {
		threshold = 0
	}

	req := sandbox.CommandRequest{
		Args:    []string{"go", "test", "-cover", pkg},
		Workdir: workspaceRoot(h.env),
		Timeout: 5 * time.Minute,
	}

	if err := h.authorizeCommand(ctx, h.env, req, "euclo coverage check"); err != nil {
		return failResult(fmt.Sprintf("coverage command denied: %v", err)), err
	}

	res, err := h.env.CommandRunner.Run(ctx, req)
	if err != nil {
		return nil, err
	}
	combined := res.Stdout + res.Stderr

	coverage, packages := parseCoverageOutput(combined)

	totalCoverage := 0.0
	if len(packages) > 0 {
		sum := 0.0
		for _, p := range packages {
			sum += p.Coverage
		}
		totalCoverage = sum / float64(len(packages))
	}

	passed := res.ExitCode == 0 && (threshold == 0 || totalCoverage >= threshold)

	return &ports.CapabilityExecutionResult{
		Success: true,
		Data: map[string]interface{}{
			"success":        true,
			"passed":         passed,
			"packages":       coveragePackagesToInterfaces(packages),
			"coverage":       coverage,
			"total_coverage": totalCoverage,
			"threshold":      threshold,
			"output":         truncate(combined, 8192),
			"exit_code":      res.ExitCode,
			"package":        pkg,
		},
	}, nil
}
