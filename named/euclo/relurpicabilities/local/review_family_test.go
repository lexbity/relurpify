package local

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

type stubSurfaceExtractor struct{}

func (stubSurfaceExtractor) ExtractSurface(_ context.Context, _ agentenv.CompatibilitySurfaceRequest) (agentenv.CompatibilitySurface, bool, error) {
	return agentenv.CompatibilitySurface{
		Functions: []map[string]any{{"name": "External", "location": "api.go:1"}},
		Metadata:  map[string]any{"source": "stub"},
	}, true, nil
}

func TestSemanticReviewFlagsBreakingPublicSurfaceChange(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.code", map[string]any{
		"summary": "remove exported API",
		"compatibility_after_surface": map[string]any{
			"functions": []map[string]any{},
			"types":     []map[string]any{},
		},
	})
	env := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			Instruction: "review this API compatibility change",
			Context: map[string]any{
				"context_file_contents": []map[string]any{{
					"path":    "api.go",
					"content": "package sample\n\nfunc Exported(input string) string { return input }\n",
				}},
			},
		},
		State: state,
	}

	payload := buildSemanticReviewPayload(env)
	stats, _ := payload["stats"].(map[string]any)
	if intValue(stats["critical_count"]) == 0 {
		t.Fatalf("expected critical findings, got %#v", payload)
	}
	approval, _ := payload["approval_decision"].(map[string]any)
	if stringValue(approval["status"]) != "blocked" {
		t.Fatalf("expected blocked approval, got %#v", approval)
	}
}

func TestSemanticReviewWarnsWhenVerificationEvidenceRequiredButMissing(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.resolved_execution_policy", eucloruntime.ResolvedExecutionPolicy{
		ReviewApprovalRules: core.AgentReviewApprovalRules{
			RequireVerificationEvidence: true,
		},
	})
	state.Set("pipeline.code", map[string]any{
		"summary": "adjust correctness handling in run",
	})
	env := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			Instruction: "review correctness of this change",
			Context: map[string]any{
				"context_file_contents": []map[string]any{{
					"path":    "main.go",
					"content": "package main\n\nfunc run() error { return nil }\n",
				}},
			},
		},
		State: state,
	}

	payload := buildSemanticReviewPayload(env)
	findings := findingsFromPayload(payload)
	found := false
	for _, finding := range findings {
		if stringValue(finding["category"]) == "verification_coverage" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected verification coverage warning, got %#v", findings)
	}
}

func TestReviewImplementIfSafeBlocksOnSemanticCriticalFindings(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.code", map[string]any{
		"summary": "remove exported API",
		"compatibility_after_surface": map[string]any{
			"functions": []map[string]any{},
			"types":     []map[string]any{},
		},
	})
	env := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "review-safe",
			Instruction: "implement if safe and fix findings for this API change",
			Context: map[string]any{
				"context_file_contents": []map[string]any{{
					"path":    "api.go",
					"content": "package sample\n\nfunc Exported(input string) string { return input }\n",
				}},
			},
		},
		State: state,
	}

	result := NewReviewImplementIfSafeCapability(agentenv.AgentEnvironment{}).Execute(context.Background(), env)
	if result.RecoveryHint == nil {
		t.Fatalf("expected recovery hint, got %#v", result)
	}
	if len(result.Artifacts) == 0 {
		t.Fatalf("expected review artifacts, got %#v", result)
	}
}

func TestSemanticReviewKeepsCleanCodeAtInfoOnly(t *testing.T) {
	env := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			Instruction: "review this small helper change",
			Context: map[string]any{
				"context_file_contents": []map[string]any{{
					"path":    "helper.go",
					"content": "package sample\n\nfunc helper(input string) string { return input }\n",
				}},
			},
		},
		State: core.NewContext(),
	}

	payload := buildSemanticReviewPayload(env)
	stats, _ := payload["stats"].(map[string]any)
	if intValue(stats["critical_count"]) != 0 || intValue(stats["warning_count"]) != 0 {
		t.Fatalf("expected clean code to avoid severe findings, got %#v", payload)
	}
}

func TestExtractAPISurfaceWithEnv_UsesExternalExtractor(t *testing.T) {
	surface := extractAPISurfaceWithEnv(agentenv.AgentEnvironment{
		CompatibilitySurfaceExtractor: stubSurfaceExtractor{},
	}, "review this API", []reviewFile{{Path: "api.go", Content: "package sample"}})
	functions := surfaceItems(surface["functions"])
	if len(functions) != 1 || stringValue(functions[0]["name"]) != "External" {
		t.Fatalf("expected external extractor surface, got %#v", surface)
	}
}

func findingsFromPayload(payload map[string]any) []map[string]any {
	raw, ok := payload["findings"]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []map[string]any:
		return typed
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if record, ok := item.(map[string]any); ok {
				out = append(out, record)
			}
		}
		return out
	default:
		return nil
	}
}
