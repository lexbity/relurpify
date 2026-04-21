package python

import (
	"context"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
)

type VerificationResolver struct{}

func NewVerificationResolver() *VerificationResolver {
	return &VerificationResolver{}
}

func (r *VerificationResolver) BackendID() string { return "python" }

func (r *VerificationResolver) Supports(req agentenv.VerificationPlanRequest) bool {
	for _, file := range append(append([]string(nil), req.Files...), req.TestFiles...) {
		path := strings.ToLower(strings.TrimSpace(file))
		if strings.HasSuffix(path, ".py") || strings.HasSuffix(path, "pyproject.toml") || strings.HasSuffix(path, "pytest.ini") || strings.HasSuffix(path, "requirements.txt") {
			return true
		}
	}
	for _, capability := range append(append([]string(nil), req.PreferredVerifyCapabilities...), req.VerificationSuccessCapabilities...) {
		lower := strings.ToLower(strings.TrimSpace(capability))
		if strings.Contains(lower, "pytest") || strings.Contains(lower, "python") {
			return true
		}
	}
	lowerTask := strings.ToLower(strings.TrimSpace(req.TaskInstruction))
	return strings.Contains(lowerTask, "python") || strings.Contains(lowerTask, "pytest")
}

func (r *VerificationResolver) BuildPlan(_ context.Context, req agentenv.VerificationPlanRequest) (agentenv.VerificationPlan, bool, error) {
	workspace := firstNonEmpty(strings.TrimSpace(req.Workspace), ".")
	commands := []agentenv.VerificationCommand{}
	scopeKind := "workspace_tests"
	if len(req.TestFiles) > 0 {
		scopeKind = "test_files"
		args := append([]string{"-m", "pytest", "-q"}, req.TestFiles...)
		commands = append(commands, agentenv.VerificationCommand{
			Name:             "python_pytest_targeted",
			Command:          "python",
			Args:             args,
			WorkingDirectory: workspace,
		})
	}
	if len(commands) == 0 || req.PublicSurfaceChanged || req.RequireVerificationStep {
		commands = append(commands, agentenv.VerificationCommand{
			Name:             "python_pytest",
			Command:          "python",
			Args:             []string{"-m", "pytest", "-q"},
			WorkingDirectory: workspace,
		})
	}
	if req.PublicSurfaceChanged {
		scopeKind = "compatibility_sweep"
	}
	return agentenv.VerificationPlan{
		ScopeKind:              scopeKind,
		Files:                  uniqueStrings(append(append([]string(nil), req.Files...), req.TestFiles...)),
		TestFiles:              uniqueStrings(req.TestFiles),
		Commands:               uniqueCommands(commands),
		Source:                 "platform.lang.python",
		PlannerID:              "platform.lang.python.verification",
		Rationale:              pythonVerificationRationale(req),
		AuditTrail:             []string{"pytest_scope"},
		CompatibilitySensitive: req.PublicSurfaceChanged,
		Metadata: map[string]any{
			"test_targets": append([]string(nil), req.TestFiles...),
		},
	}, true, nil
}

func pythonVerificationRationale(req agentenv.VerificationPlanRequest) string {
	parts := []string{"pytest was selected as the default Python verification backend"}
	if len(req.TestFiles) > 0 {
		parts = append(parts, "test file changes were included in scope selection")
	}
	if req.PublicSurfaceChanged {
		parts = append(parts, "public surface changed, so verification breadth was increased")
	}
	if req.RequireVerificationStep {
		parts = append(parts, "skill policy required an explicit verification step")
	}
	return strings.Join(parts, "; ")
}

func uniqueStrings(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(input))
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func uniqueCommands(input []agentenv.VerificationCommand) []agentenv.VerificationCommand {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]agentenv.VerificationCommand, 0, len(input))
	for _, cmd := range input {
		key := cmd.Command + "\x00" + strings.Join(cmd.Args, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cmd)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
