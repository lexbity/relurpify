package rust

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

func (r *VerificationResolver) BackendID() string { return "rust" }

func (r *VerificationResolver) Supports(req contracts.VerificationPlanRequest) bool {
	for _, file := range append(append([]string(nil), req.Files...), req.TestFiles...) {
		path := strings.ToLower(strings.TrimSpace(file))
		if strings.HasSuffix(path, ".rs") || strings.HasSuffix(path, "cargo.toml") {
			return true
		}
	}
	for _, capability := range append(append([]string(nil), req.PreferredVerifyCapabilities...), req.VerificationSuccessCapabilities...) {
		lower := strings.ToLower(strings.TrimSpace(capability))
		if strings.Contains(lower, "cargo") || strings.Contains(lower, "rust") {
			return true
		}
	}
	lowerTask := strings.ToLower(strings.TrimSpace(req.TaskInstruction))
	return strings.Contains(lowerTask, "cargo") || strings.Contains(lowerTask, "rust")
}

func (r *VerificationResolver) BuildPlan(_ context.Context, req contracts.VerificationPlanRequest) (contracts.VerificationPlan, bool, error) {
	workspace := firstNonEmpty(strings.TrimSpace(req.Workspace), ".")
	commands := []contracts.VerificationCommand{{
		Command: []string{"cargo", "test"},
		Dir:     workspace,
	}}
	scopeKind := "workspace_tests"
	if req.PublicSurfaceChanged || req.RequireVerificationStep {
		commands = append(commands, contracts.VerificationCommand{
			Command: []string{"cargo", "check"},
			Dir:     workspace,
		})
	}
	if req.PublicSurfaceChanged {
		scopeKind = "compatibility_sweep"
	}
	return contracts.VerificationPlan{
		ScopeKind:              scopeKind,
		Files:                  uniqueStrings(append(append([]string(nil), req.Files...), req.TestFiles...)),
		TestFiles:              uniqueStrings(req.TestFiles),
		Commands:               uniqueCommands(commands),
		Source:                 "platform.lang.rust",
		PlannerID:              "platform.lang.rust.verification",
		Rationale:              rustVerificationRationale(req),
		AuditTrail:             []string{"cargo_scope"},
		CompatibilitySensitive: req.PublicSurfaceChanged,
		Metadata: map[string]any{
			"workspace_rooted": true,
		},
	}, true, nil
}

func rustVerificationRationale(req contracts.VerificationPlanRequest) string {
	parts := []string{"cargo test was selected as the default Rust verification backend"}
	if req.PublicSurfaceChanged {
		parts = append(parts, "public surface changed, so cargo check was added")
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
