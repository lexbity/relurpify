package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkillSpecValidation_Valid(t *testing.T) {
	yaml := `
apiVersion: relurpify/v1alpha1
kind: SkillManifest
metadata:
  name: testskill
  version: 1.0.0
  description: A test skill.
spec:
  requires:
    bins: [go]
  prompt_snippets:
    - "Use file tools before editing."
  allowed_tools:
    - file_read
    - cli_go
  tool_execution_policy:
    cli_go:
      execute: ask
  phase_tools:
    verify: [cli_go]
  phase_selectors:
    explore:
      - tags: [search]
      - tool: file_read
  verification:
    success_tools: [cli_go]
    success_selectors:
      - tags: [test]
    stop_on_success: true
  recovery:
    failure_probe_tools: [file_read]
    failure_probe_selectors:
      - tags: [recovery]
  planning:
    required_before_edit:
      - tags: [workspace-detect]
    preferred_verify_tools:
      - tags: [test]
    step_templates:
      - kind: discover
        description: Identify active project context
    require_verification_step: true
  review:
    criteria: [correctness, completeness]
    focus_tags: [verification]
    approval_rules:
      require_verification_evidence: true
      reject_on_unresolved_errors: true
    severity_weights:
      high: 1.0
`
	f := writeTempSkillFile(t, yaml)
	m, err := LoadSkillManifest(f)
	if err != nil {
		t.Fatalf("expected valid skill, got error: %v", err)
	}
	if m.Metadata.Name != "testskill" {
		t.Errorf("expected name 'testskill', got %q", m.Metadata.Name)
	}
	if len(m.Spec.Requires.Bins) != 1 || m.Spec.Requires.Bins[0] != "go" {
		t.Errorf("expected bins=[go], got %v", m.Spec.Requires.Bins)
	}
	if len(m.Spec.AllowedTools) != 2 {
		t.Errorf("expected 2 allowed_tools, got %d", len(m.Spec.AllowedTools))
	}
	if len(m.Spec.ToolExecutionPolicy) != 1 {
		t.Errorf("expected 1 tool_execution_policy entry, got %d", len(m.Spec.ToolExecutionPolicy))
	}
	if len(m.Spec.PhaseTools["verify"]) != 1 {
		t.Errorf("expected verify phase tool config, got %v", m.Spec.PhaseTools)
	}
	if len(m.Spec.PhaseSelectors["explore"]) != 2 {
		t.Errorf("expected explore phase selectors, got %v", m.Spec.PhaseSelectors)
	}
	if !m.Spec.Verification.StopOnSuccess {
		t.Errorf("expected verification.stop_on_success=true")
	}
	if len(m.Spec.Verification.SuccessSelectors) != 1 {
		t.Errorf("expected verification.success_selectors, got %v", m.Spec.Verification.SuccessSelectors)
	}
	if len(m.Spec.Planning.RequiredBeforeEdit) != 1 || !m.Spec.Planning.RequireVerificationStep {
		t.Errorf("expected planning config, got %+v", m.Spec.Planning)
	}
	if len(m.Spec.Review.Criteria) != 2 || !m.Spec.Review.ApprovalRules.RequireVerificationEvidence {
		t.Errorf("expected review config, got %+v", m.Spec.Review)
	}
}

func TestSkillSpecValidation_NoBins(t *testing.T) {
	yaml := `
apiVersion: relurpify/v1alpha1
kind: SkillManifest
metadata:
  name: nobins
  version: 1.0.0
spec:
  allowed_tools:
    - file_read
`
	f := writeTempSkillFile(t, yaml)
	m, err := LoadSkillManifest(f)
	if err != nil {
		t.Fatalf("skill with no requires.bins should be valid, got error: %v", err)
	}
	if len(m.Spec.Requires.Bins) != 0 {
		t.Errorf("expected empty bins, got %v", m.Spec.Requires.Bins)
	}
}

func TestSkillSpecValidation_SlashInBin(t *testing.T) {
	yaml := `
apiVersion: relurpify/v1alpha1
kind: SkillManifest
metadata:
  name: badskill
  version: 1.0.0
spec:
  requires:
    bins: [/usr/bin/go]
`
	f := writeTempSkillFile(t, yaml)
	_, err := LoadSkillManifest(f)
	if err == nil {
		t.Fatal("expected error for bin with '/', got nil")
	}
}

func TestSkillSpecValidation_EmptyBin(t *testing.T) {
	yaml := `
apiVersion: relurpify/v1alpha1
kind: SkillManifest
metadata:
  name: emptybin
  version: 1.0.0
spec:
  requires:
    bins: [""]
`
	f := writeTempSkillFile(t, yaml)
	_, err := LoadSkillManifest(f)
	if err == nil {
		t.Fatal("expected error for empty bin name, got nil")
	}
}

func TestLoadSkillFlat(t *testing.T) {
	ws := t.TempDir()
	skillDir := filepath.Join(ws, skillsDirName, "mypkg")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `
apiVersion: relurpify/v1alpha1
kind: SkillManifest
metadata:
  name: mypkg
  version: 1.0.0
spec:
  requires:
    bins: [python]
  allowed_tools:
    - cli_python
`
	manifestPath := filepath.Join(skillDir, "skill.manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	m, err := LoadSkill(ws, "mypkg")
	if err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}
	if m.Metadata.Name != "mypkg" {
		t.Errorf("expected name 'mypkg', got %q", m.Metadata.Name)
	}
	if m.SourcePath != manifestPath {
		t.Errorf("expected SourcePath %q, got %q", manifestPath, m.SourcePath)
	}
	if len(m.Spec.Requires.Bins) != 1 || m.Spec.Requires.Bins[0] != "python" {
		t.Errorf("expected bins=[python], got %v", m.Spec.Requires.Bins)
	}
}

func TestLoadSkillFlat_Missing(t *testing.T) {
	ws := t.TempDir()
	_, err := LoadSkill(ws, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing skill, got nil")
	}
}

func TestLoadSkillList_PartialLoad(t *testing.T) {
	ws := t.TempDir()
	skillDir := filepath.Join(ws, skillsDirName, "goodskill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `
apiVersion: relurpify/v1alpha1
kind: SkillManifest
metadata:
  name: goodskill
  version: 1.0.0
spec:
  allowed_tools:
    - file_read
`
	if err := os.WriteFile(filepath.Join(skillDir, "skill.manifest.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	loaded := LoadSkillList(ws, []string{"goodskill", "missing_skill"})
	if len(loaded) != 1 {
		t.Errorf("expected 1 loaded skill, got %d", len(loaded))
	}
	if loaded[0].Metadata.Name != "goodskill" {
		t.Errorf("expected 'goodskill', got %q", loaded[0].Metadata.Name)
	}
}

func TestRepositoryLanguageSkillsCarryPlanningAndReviewPolicy(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	skills := []string{
		"ccoder",
		"gocoder",
		"nodecoder",
		"pycoder",
		"rustcoder",
		"sqlcoder",
	}
	for _, skill := range skills {
		skill := skill
		t.Run(skill, func(t *testing.T) {
			path := filepath.Join(root, "relurpify_cfg", "skills", skill, "skill.manifest.yaml")
			m, err := LoadSkillManifest(path)
			if err != nil {
				t.Fatalf("load manifest: %v", err)
			}
			if len(m.Spec.Planning.StepTemplates) == 0 {
				t.Fatalf("expected planning.step_templates in %s", skill)
			}
			if !m.Spec.Planning.RequireVerificationStep {
				t.Fatalf("expected planning.require_verification_step in %s", skill)
			}
			if len(m.Spec.Review.Criteria) == 0 {
				t.Fatalf("expected review.criteria in %s", skill)
			}
			if !m.Spec.Review.ApprovalRules.RequireVerificationEvidence {
				t.Fatalf("expected review.approval_rules.require_verification_evidence in %s", skill)
			}
		})
	}
}

// writeTempSkillFile writes yaml content to a temp file and returns its path.
func writeTempSkillFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "skill-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}
