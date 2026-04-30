package orchestrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/manifest"
)

func TestDispatcherExecuteSkillFilterNarrowsCandidates(t *testing.T) {
	workspace := t.TempDir()
	writeSkillManifestFixture(t, workspace, "bundle", []string{
		"euclo:cap.ast_query",
		"euclo:cap.symbol_trace",
	})

	reg := capability.NewCapabilityRegistry()
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
	env.SetWorkingValue("euclo.skill_filter", "bundle", contextdata.MemoryClassTask)

	result, err := dispatcher.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if got, ok := env.GetWorkingValue("euclo.route.skill_filter"); !ok || got != "bundle" {
		t.Fatalf("expected skill filter recorded in envelope, got %v (ok=%v)", got, ok)
	}
	if got, ok := env.GetWorkingValue("euclo.route.candidate_count"); !ok || got != 2 {
		t.Fatalf("expected 2 allowed candidates, got %v (ok=%v)", got, ok)
	}
	if result.Data["skill_filter"] != "bundle" {
		t.Fatalf("expected skill filter in result, got %v", result.Data["skill_filter"])
	}
}

func TestDispatcherExecuteSkillFilterUnknownSkill(t *testing.T) {
	workspace := t.TempDir()
	dispatcher := NewDispatcher("dispatcher1").
		WithWorkspace(workspace).
		WithCapabilityRegistry(capability.NewCapabilityRegistry())
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("euclo.skill_filter", "missing-skill", contextdata.MemoryClassTask)

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
		WithCapabilityRegistry(capability.NewCapabilityRegistry())
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("euclo.skill_filter", "empty-bundle", contextdata.MemoryClassTask)

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

	reg := capability.NewCapabilityRegistry()
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
	root := filepath.Join(manifest.New(workspace).SkillsDir(), name)
	for _, dir := range []string{"scripts", "resources", "templates"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	var builder strings.Builder
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
	path := filepath.Join(root, "skill.yaml")
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		t.Fatalf("write skill manifest: %v", err)
	}
	return root
}
