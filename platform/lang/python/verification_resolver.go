package python

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type VerificationResolver struct{}

func NewVerificationResolver() *VerificationResolver {
	return &VerificationResolver{}
}

func (r *VerificationResolver) BackendID() string { return "python" }

func (r *VerificationResolver) Supports(req contracts.VerificationPlanRequest) bool {
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

func (r *VerificationResolver) BuildPlan(_ context.Context, req contracts.VerificationPlanRequest) (contracts.VerificationPlan, bool, error) {
	workspace := firstNonEmpty(strings.TrimSpace(req.Workspace), ".")
	commands := []contracts.VerificationCommand{}
	if len(req.TestFiles) > 0 {
		args := append([]string{"-m", "pytest", "-q"}, req.TestFiles...)
		commands = append(commands, contracts.VerificationCommand{
			Command: append([]string{"python"}, args...),
			Dir:     workspace,
		})
	}
	if len(commands) == 0 || req.PublicSurfaceChanged || req.RequireVerificationStep {
		commands = append(commands, contracts.VerificationCommand{
			Command: []string{"python", "-m", "pytest", "-q"},
			Dir:     workspace,
		})
	}
	return contracts.VerificationPlan{
		Commands:  uniqueCommands(commands),
		Rationale: pythonVerificationRationale(req),
	}, true, nil
}

func pythonVerificationRationale(req contracts.VerificationPlanRequest) string {
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

func uniqueCommands(input []contracts.VerificationCommand) []contracts.VerificationCommand {
	seen := make(map[string]struct{}, len(input))
	out := make([]contracts.VerificationCommand, 0, len(input))
	for _, cmd := range input {
		key := fmt.Sprintf("%s:%v", cmd.Command, cmd.Args)
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
