package orchestrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

func TestDispatcherExecuteSkillFilterNarrowsCandidates(t *testing.T) {
	workspace := t.TempDir()
	writeSkillManifestFixture(t, workspace, "bundle", []string{
		"euclo:cap.ast_query",
		"euclo:cap.symbol_trace",
	})

	reg := capability.NewRegistry()
	for _, entry := range []struct {
		id       string
		priority int
	}{
		{id: "euclo:cap.ast_query", priority: 5},
		{id: "euclo:cap.symbol_trace", priority: 10},
		{id: "euclo:cap.code_review", priority: 15},
	} {
		if err := reg.RegisterCapability(testCapabilityDescriptor(entry.id, entry.priority, core.AvailabilitySpec{Available: true})); err != nil {
			t.Fatalf("register capability %s: %v", entry.id, err)
		}
	}

	dispatcher := NewDispatcher("dispatcher1").
		WithWorkspace(workspace).
		WithCapabilityRegistry(reg)
	env := contextdata.NewEnvelope("task-1", "session-1")
	state.SetSkillFilter(env, "bundle")

	result, err := dispatcher.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if got := state.GetRouteSkillFilter(env); got != "bundle" {
		t.Fatalf("expected skill filter recorded in envelope, got %v", got)
	}
	if got, ok := state.GetRouteCandidateCount(env); !ok || got != 2 {
		t.Fatalf("expected 2 allowed candidates, got %v (ok=%v)", got, ok)
	}
	if got, ok := core.ResultField(result.Data, "skill_filter"); !ok || got != "bundle" {
		t.Fatalf("expected skill filter in result, got %v", got)
	}
}

func TestDispatcherExecuteSkillFilterUnknownSkill(t *testing.T) {
	workspace := t.TempDir()
	dispatcher := NewDispatcher("dispatcher1").
		WithWorkspace(workspace).
		WithCapabilityRegistry(capability.NewRegistry())
	env := contextdata.NewEnvelope("task-1", "session-1")
	state.SetSkillFilter(env, "missing-skill")

	_, err := dispatcher.Execute(context.Background(), env)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing-skill") {
		t.Fatalf("expected error to mention missing-skill, got %v", err)
	}
}

func TestDispatcherExecuteSkillFilterEmptyAllowedCapabilities(t *testing.T) {
	workspace := t.TempDir()
	writeSkillManifestFixture(t, workspace, "empty-bundle", nil)

	dispatcher := NewDispatcher("dispatcher1").
		WithWorkspace(workspace).
		WithCapabilityRegistry(capability.NewRegistry())
	env := contextdata.NewEnvelope("task-1", "session-1")
	state.SetSkillFilter(env, "empty-bundle")

	_, err := dispatcher.Execute(context.Background(), env)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no allowed capabilities") {
		t.Fatalf("expected empty allowed capability error, got %v", err)
	}
}

func TestSkillFilterDryRunRespectsAvailability(t *testing.T) {
	workspace := t.TempDir()
	writeSkillManifestFixture(t, workspace, "bundle", []string{"euclo:cap.targeted_refactor"})

	reg := capability.NewRegistry()
	if err := reg.RegisterCapability(testCapabilityDescriptor("euclo:cap.targeted_refactor", 10, core.AvailabilitySpec{
		Available: false,
		Reason:    "tool dependency missing: file_write",
	})); err != nil {
		t.Fatalf("register unavailable capability: %v", err)
	}
	if err := reg.RegisterCapability(testCapabilityDescriptor("euclo:cap.code_review", 5, core.AvailabilitySpec{Available: true})); err != nil {
		t.Fatalf("register extra capability: %v", err)
	}

	scoped, err := applySkillFilterToRegistry(workspace, "bundle", reg)
	if err != nil {
		t.Fatalf("applySkillFilterToRegistry failed: %v", err)
	}

	report, dryErr := DryRun(context.Background(), contextdata.NewEnvelope("task-1", "session-1"), RouteRequest{
		SkillFilter: "bundle",
		DryRun:      true,
	}, scoped, nil)
	if report == nil {
		t.Fatal("expected dry-run report")
	}
	if len(report.Candidates) != 1 {
		t.Fatalf("expected 1 candidate after skill filtering, got %d", len(report.Candidates))
	}
	if report.Candidates[0].Availability == RouteAvailable {
		t.Fatalf("expected candidate to be unavailable, got %+v", report.Candidates[0])
	}
	if dryErr == nil {
		t.Fatal("expected dry-run error for unavailable candidate")
	}
}

func writeSkillManifestFixture(t *testing.T, workspace, name string, allowed []string) string {
	t.Helper()
	root := cfgload.New(workspace).SkillsDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir skill root: %v", err)
	}
	var builder strings.Builder
	builder.WriteString("schema: relurpify/skill/v1\n")
	builder.WriteString("apiVersion: euclo.skills/v1\n")
	builder.WriteString("kind: SkillManifest\n")
	builder.WriteString("metadata:\n")
	builder.WriteString("  name: " + name + "\n")
	if len(allowed) > 0 {
		builder.WriteString("spec:\n")
		builder.WriteString("  allowed_capabilities:\n")
		for _, capID := range allowed {
			builder.WriteString("    - id: " + capID + "\n")
		}
	}
	path := filepath.Join(root, name+".skill.yaml")
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		t.Fatalf("write skill manifest: %v", err)
	}
	return root
}
