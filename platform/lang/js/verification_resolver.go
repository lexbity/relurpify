package js

import (
	"context"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
)

type VerificationResolver struct{}

func NewVerificationResolver() *VerificationResolver {
	return &VerificationResolver{}
}

func (r *VerificationResolver) BackendID() string { return "javascript" }

func (r *VerificationResolver) Supports(req agentenv.VerificationPlanRequest) bool {
	for _, file := range append(append([]string(nil), req.Files...), req.TestFiles...) {
		path := strings.ToLower(strings.TrimSpace(file))
		switch {
		case strings.HasSuffix(path, ".js"), strings.HasSuffix(path, ".jsx"), strings.HasSuffix(path, ".ts"), strings.HasSuffix(path, ".tsx"),
			strings.HasSuffix(path, "package.json"), strings.HasSuffix(path, "pnpm-lock.yaml"), strings.HasSuffix(path, "package-lock.json"), strings.HasSuffix(path, "yarn.lock"):
			return true
		}
	}
	for _, capability := range append(append([]string(nil), req.PreferredVerifyCapabilities...), req.VerificationSuccessCapabilities...) {
		lower := strings.ToLower(strings.TrimSpace(capability))
		if strings.Contains(lower, "npm") || strings.Contains(lower, "node") || strings.Contains(lower, "js") || strings.Contains(lower, "jest") || strings.Contains(lower, "vitest") {
			return true
		}
	}
	lowerTask := strings.ToLower(strings.TrimSpace(req.TaskInstruction))
	return strings.Contains(lowerTask, "javascript") || strings.Contains(lowerTask, "typescript") || strings.Contains(lowerTask, "node") || strings.Contains(lowerTask, "npm") || strings.Contains(lowerTask, "jest") || strings.Contains(lowerTask, "vitest")
}

func (r *VerificationResolver) BuildPlan(_ context.Context, req agentenv.VerificationPlanRequest) (agentenv.VerificationPlan, bool, error) {
	workspace := jsResolverFirstNonEmpty(strings.TrimSpace(req.Workspace), ".")
	scopeKind := "workspace_tests"
	if req.PublicSurfaceChanged {
		scopeKind = "compatibility_sweep"
	}
	return agentenv.VerificationPlan{
		ScopeKind: scopeKind,
		Files:     uniqueStrings(append(append([]string(nil), req.Files...), req.TestFiles...)),
		TestFiles: uniqueStrings(req.TestFiles),
		Commands: []agentenv.VerificationCommand{{
			Name:             "npm_test",
			Command:          "npm",
			Args:             []string{"test"},
			WorkingDirectory: workspace,
		}},
		Source:                 "platform.lang.js",
		PlannerID:              "platform.lang.js.verification",
		Rationale:              jsVerificationRationale(req),
		AuditTrail:             []string{"npm_test_scope"},
		CompatibilitySensitive: req.PublicSurfaceChanged,
		Metadata: map[string]any{
			"workspace_rooted": true,
		},
	}, true, nil
}

func jsVerificationRationale(req agentenv.VerificationPlanRequest) string {
	parts := []string{"npm test was selected as the default JavaScript/TypeScript verification backend"}
	if req.PublicSurfaceChanged {
		parts = append(parts, "public surface changed, so the plan was marked compatibility-sensitive")
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

func jsResolverFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
