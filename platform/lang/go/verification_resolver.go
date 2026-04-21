package golang

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
)

type VerificationResolver struct{}

func NewVerificationResolver() *VerificationResolver {
	return &VerificationResolver{}
}

func (r *VerificationResolver) BackendID() string { return "go" }

func (r *VerificationResolver) Supports(req agentenv.VerificationPlanRequest) bool {
	for _, file := range append(append([]string(nil), req.Files...), req.TestFiles...) {
		path := strings.ToLower(strings.TrimSpace(file))
		if strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "go.mod") {
			return true
		}
	}
	for _, capability := range append(append([]string(nil), req.PreferredVerifyCapabilities...), req.VerificationSuccessCapabilities...) {
		if strings.Contains(strings.ToLower(strings.TrimSpace(capability)), "go") {
			return true
		}
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(req.TaskInstruction)), "go")
}

func (r *VerificationResolver) BuildPlan(_ context.Context, req agentenv.VerificationPlanRequest) (agentenv.VerificationPlan, bool, error) {
	packages := goVerificationPackages(req.Files, req.TestFiles)
	commands := buildGoVerificationCommands(packages, req)
	if len(commands) == 0 {
		return agentenv.VerificationPlan{}, false, nil
	}
	scopeKind := "package_tests"
	if len(packages) == 0 {
		scopeKind = "workspace_tests"
	}
	if req.PublicSurfaceChanged {
		scopeKind = "compatibility_sweep"
	}
	return agentenv.VerificationPlan{
		ScopeKind:              scopeKind,
		Files:                  uniqueStrings(append(append([]string(nil), req.Files...), req.TestFiles...)),
		TestFiles:              uniqueStrings(req.TestFiles),
		Commands:               commands,
		Source:                 "platform.lang.go",
		PlannerID:              "platform.lang.go.verification",
		Rationale:              goVerificationRationale(req, len(packages) > 0),
		AuditTrail:             []string{"package_scope"},
		CompatibilitySensitive: req.PublicSurfaceChanged,
		Metadata: map[string]any{
			"package_targets": append([]string(nil), packages...),
		},
	}, true, nil
}

func goVerificationPackages(files, testFiles []string) []string {
	packages := []string{}
	for _, file := range append(append([]string(nil), files...), testFiles...) {
		path := strings.TrimSpace(file)
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		switch dir {
		case "", ".":
			packages = append(packages, ".")
		default:
			packages = append(packages, "./"+strings.TrimPrefix(dir, "./"))
		}
	}
	return uniqueStrings(packages)
}

func buildGoVerificationCommands(packages []string, req agentenv.VerificationPlanRequest) []agentenv.VerificationCommand {
	workspace := firstNonEmpty(strings.TrimSpace(req.Workspace), ".")
	if len(packages) == 0 {
		return []agentenv.VerificationCommand{{
			Name:             "go_test_all",
			Command:          "go",
			Args:             []string{"test", "./..."},
			WorkingDirectory: workspace,
		}}
	}
	commands := make([]agentenv.VerificationCommand, 0, len(packages)+1)
	for _, pkg := range packages {
		commands = append(commands, agentenv.VerificationCommand{
			Name:             "go_test_" + sanitizeName(pkg),
			Command:          "go",
			Args:             []string{"test", pkg},
			WorkingDirectory: workspace,
		})
	}
	if req.PublicSurfaceChanged || req.RequireVerificationStep {
		commands = append(commands, agentenv.VerificationCommand{
			Name:             "go_test_all",
			Command:          "go",
			Args:             []string{"test", "./..."},
			WorkingDirectory: workspace,
		})
	}
	return uniqueCommands(commands)
}

func goVerificationRationale(req agentenv.VerificationPlanRequest, targeted bool) string {
	parts := []string{}
	if targeted {
		parts = append(parts, "targeted Go package tests were derived from changed files")
	} else {
		parts = append(parts, "workspace-wide Go tests were selected")
	}
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
		key := fmt.Sprintf("%s\x00%s", cmd.Command, strings.Join(cmd.Args, "\x00"))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cmd)
	}
	return out
}

func sanitizeName(value string) string {
	replacer := strings.NewReplacer("/", "_", ".", "_", "-", "_")
	value = replacer.Replace(strings.TrimSpace(value))
	if value == "" {
		return "target"
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
