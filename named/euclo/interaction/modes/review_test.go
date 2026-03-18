package modes

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

func TestReviewMode_FullFlow_NoFindings(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	resolver := interaction.NewAgencyResolver()
	m := ReviewMode(emitter, resolver)

	m.State()["instruction"] = "Review recent changes"
	m.State()["scope"] = []string{"handler.go"}

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// No findings → triage marks no_fixes → act and re_review skipped.
	if v, _ := m.State()["triage.no_fixes"].(bool); !v {
		t.Error("expected triage.no_fixes to be true")
	}
}

func TestReviewMode_WithFindings(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	resolver := interaction.NewAgencyResolver()
	m := ReviewMode(emitter, resolver)

	m.State()["instruction"] = "Review code"

	// Inject a sweep phase that produces findings.
	m = interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:    "review",
		Emitter: emitter,
		Phases: []interaction.PhaseDefinition{
			{ID: "scope", Label: "Scope", Handler: &ReviewScopePhase{}},
			{ID: "sweep", Label: "Sweep", Handler: &ReviewSweepPhase{
				RunReview: func(_ context.Context, _ interaction.PhaseMachineContext) (interaction.FindingsContent, error) {
					return interaction.FindingsContent{
						Critical: []interaction.Finding{{Location: "auth.go:15", Description: "SQL injection"}},
						Warning:  []interaction.Finding{{Location: "handler.go:80", Description: "Missing check"}},
					}, nil
				},
			}},
			{ID: "triage", Label: "Triage", Handler: &TriagePhase{}},
			{ID: "act", Label: "Act", Handler: &BatchFixPhase{}, Skippable: true, SkipWhen: skipAct},
			{ID: "re_review", Label: "Re-review", Handler: &ReReviewPhase{}, Skippable: true, SkipWhen: skipReReview},
		},
	})

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// With findings, triage should have emitted a result frame with findings.
	var foundTriageResult bool
	for _, f := range emitter.Frames {
		if f.Phase == "triage" && f.Kind == interaction.FrameResult {
			foundTriageResult = true
		}
	}
	if !foundTriageResult {
		t.Error("expected triage result frame")
	}
}

func TestReviewScopePhase_Default(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &ReviewScopePhase{}

	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"instruction": "Review auth module",
			"scope":       []string{"auth.go", "auth_test.go"},
		},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "review",
		Phase:      "scope",
		PhaseIndex: 0,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !outcome.Advance {
		t.Error("expected advance")
	}
	if len(emitter.Frames) != 1 {
		t.Fatalf("frames: got %d, want 1", len(emitter.Frames))
	}
	if emitter.Frames[0].Kind != interaction.FrameProposal {
		t.Errorf("kind: got %q, want proposal", emitter.Frames[0].Kind)
	}
}

func TestReviewScopePhase_Compatibility(t *testing.T) {
	emitter := &fixedResponseEmitter{
		response: interaction.UserResponse{ActionID: "compat"},
	}
	phase := &ReviewScopePhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "review",
		Phase:      "scope",
		PhaseIndex: 0,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.StateUpdates["scope.check_compatibility"] != true {
		t.Error("expected check_compatibility flag")
	}
}

func TestReviewSweepPhase_Default(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &ReviewSweepPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "review",
		Phase:      "sweep",
		PhaseIndex: 1,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !outcome.Advance {
		t.Error("expected advance")
	}
	// Should emit status frame.
	if len(emitter.Frames) != 1 {
		t.Errorf("frames: got %d, want 1", len(emitter.Frames))
	}
	if emitter.Frames[0].Kind != interaction.FrameStatus {
		t.Errorf("kind: got %q, want status", emitter.Frames[0].Kind)
	}
}

func TestReviewSweepPhase_WithReview(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	called := false
	phase := &ReviewSweepPhase{
		RunReview: func(_ context.Context, _ interaction.PhaseMachineContext) (interaction.FindingsContent, error) {
			called = true
			return interaction.FindingsContent{
				Critical: []interaction.Finding{{Description: "issue"}},
			}, nil
		},
	}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "review",
		Phase:      "sweep",
		PhaseIndex: 1,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !called {
		t.Error("review callback should have been called")
	}
	findings, ok := outcome.StateUpdates["sweep.findings"].(interaction.FindingsContent)
	if !ok {
		t.Fatal("expected findings in state")
	}
	if len(findings.Critical) != 1 {
		t.Errorf("critical: got %d, want 1", len(findings.Critical))
	}
}

func TestTriagePhase_NoFindings(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &TriagePhase{}

	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"sweep.findings": interaction.FindingsContent{},
		},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "review",
		Phase:      "triage",
		PhaseIndex: 2,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if v, _ := outcome.StateUpdates["triage.no_fixes"].(bool); !v {
		t.Error("expected triage.no_fixes=true for empty findings")
	}
}

func TestTriagePhase_WithFindings(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &TriagePhase{}

	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"sweep.findings": interaction.FindingsContent{
				Critical: []interaction.Finding{{Description: "SQL injection"}},
				Warning:  []interaction.Finding{{Description: "Missing check"}},
			},
		},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "review",
		Phase:      "triage",
		PhaseIndex: 2,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Default with critical findings is "fix_critical".
	if outcome.StateUpdates["triage.response"] != "fix_critical" {
		t.Errorf("response: got %v, want 'fix_critical'", outcome.StateUpdates["triage.response"])
	}
	if outcome.StateUpdates["triage.fix_scope"] != "critical" {
		t.Errorf("fix_scope: got %v, want 'critical'", outcome.StateUpdates["triage.fix_scope"])
	}
}

func TestTriagePhase_NoFixes(t *testing.T) {
	emitter := &fixedResponseEmitter{
		response: interaction.UserResponse{ActionID: "no_fixes"},
	}
	phase := &TriagePhase{}

	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"sweep.findings": interaction.FindingsContent{
				Warning: []interaction.Finding{{Description: "minor issue"}},
			},
		},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "review",
		Phase:      "triage",
		PhaseIndex: 2,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if v, _ := outcome.StateUpdates["triage.no_fixes"].(bool); !v {
		t.Error("expected no_fixes=true")
	}
}

func TestBatchFixPhase_Default(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &BatchFixPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"triage.fix_scope": "critical",
		},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "review",
		Phase:      "act",
		PhaseIndex: 3,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !outcome.Advance {
		t.Error("expected advance")
	}
	// Status + result.
	if len(emitter.Frames) != 2 {
		t.Errorf("frames: got %d, want 2", len(emitter.Frames))
	}
	// Default is "re_review".
	if outcome.StateUpdates["act.response"] != "re_review" {
		t.Errorf("response: got %v", outcome.StateUpdates["act.response"])
	}
}

func TestBatchFixPhase_AcceptWithoutReReview(t *testing.T) {
	emitter := &fixedResponseEmitter{
		response: interaction.UserResponse{ActionID: "accept"},
	}
	phase := &BatchFixPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "review",
		Phase:      "act",
		PhaseIndex: 3,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if v, _ := outcome.StateUpdates["act.skip_re_review"].(bool); !v {
		t.Error("expected skip_re_review=true")
	}
}

func TestReReviewPhase_Clean(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &ReReviewPhase{}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "review",
		Phase:      "re_review",
		PhaseIndex: 4,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !outcome.Advance {
		t.Error("expected advance")
	}
	if outcome.StateUpdates["re_review.status"] != "passed" {
		t.Errorf("status: got %v, want 'passed'", outcome.StateUpdates["re_review.status"])
	}
}

func TestReReviewPhase_NewFindings(t *testing.T) {
	emitter := &interaction.NoopEmitter{}
	phase := &ReReviewPhase{
		RunReview: func(_ context.Context, _ interaction.PhaseMachineContext) (interaction.FindingsContent, error) {
			return interaction.FindingsContent{
				Warning: []interaction.Finding{{Description: "new warning"}},
			}, nil
		},
	}

	mc := interaction.PhaseMachineContext{
		Emitter:    emitter,
		State:      map[string]any{},
		Artifacts:  interaction.NewArtifactBundle(),
		Mode:       "review",
		Phase:      "re_review",
		PhaseIndex: 4,
		PhaseCount: 5,
	}

	outcome, err := phase.Execute(context.Background(), mc)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.StateUpdates["re_review.status"] != "partial" {
		t.Errorf("status: got %v, want 'partial'", outcome.StateUpdates["re_review.status"])
	}
}

func TestRegisterReviewTriggers(t *testing.T) {
	resolver := interaction.NewAgencyResolver()
	RegisterReviewTriggers(resolver)

	triggers := resolver.TriggersForMode("review")
	if len(triggers) != 4 {
		t.Errorf("triggers: got %d, want 4", len(triggers))
	}

	trigger, ok := resolver.Resolve("review", "fix all critical")
	if !ok {
		t.Fatal("expected fix_all_critical trigger")
	}
	if trigger.CapabilityID != "euclo:review.implement_if_safe" {
		t.Errorf("capability: got %q", trigger.CapabilityID)
	}

	trigger, ok = resolver.Resolve("review", "check compatibility")
	if !ok {
		t.Fatal("expected compatibility trigger")
	}
	if trigger.CapabilityID != "euclo:review.compatibility" {
		t.Errorf("compatibility capability: got %q", trigger.CapabilityID)
	}
}

func TestRegisterReviewTriggers_NilResolver(t *testing.T) {
	RegisterReviewTriggers(nil)
}

func TestReviewPhaseLabels(t *testing.T) {
	labels := ReviewPhaseLabels()
	if len(labels) != 5 {
		t.Fatalf("labels: got %d, want 5", len(labels))
	}
}

func TestSkipAct(t *testing.T) {
	tests := []struct {
		name  string
		state map[string]any
		want  bool
	}{
		{"no_fixes", map[string]any{"triage.no_fixes": true}, true},
		{"has_fixes", map[string]any{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skipAct(tt.state, interaction.NewArtifactBundle())
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSkipReReview(t *testing.T) {
	tests := []struct {
		name  string
		state map[string]any
		want  bool
	}{
		{"accepted_no_re_review", map[string]any{"act.skip_re_review": true}, true},
		{"no_fixes", map[string]any{"triage.no_fixes": true}, true},
		{"needs_re_review", map[string]any{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skipReReview(tt.state, interaction.NewArtifactBundle())
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildTriageActions_WithCritical(t *testing.T) {
	findings := interaction.FindingsContent{
		Critical: []interaction.Finding{{Description: "a"}},
		Warning:  []interaction.Finding{{Description: "b"}},
	}
	actions := buildTriageActions(findings)
	if len(actions) != 4 {
		t.Errorf("actions: got %d, want 4", len(actions))
	}
	if actions[0].ID != "fix_critical" || !actions[0].Default {
		t.Errorf("first action: got %q default=%v", actions[0].ID, actions[0].Default)
	}
}

func TestBuildTriageActions_NoCritical(t *testing.T) {
	findings := interaction.FindingsContent{
		Warning: []interaction.Finding{{Description: "b"}},
	}
	actions := buildTriageActions(findings)
	if len(actions) != 3 {
		t.Errorf("actions: got %d, want 3 (no fix_critical)", len(actions))
	}
	// fix_all should be default when no critical.
	if !actions[0].Default {
		t.Error("fix_all should be default when no critical findings")
	}
}
