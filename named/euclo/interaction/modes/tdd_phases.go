package modes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// TestDraftPhase presents the generated test cases as an editable draft.
type TestDraftPhase struct {
	// GenerateTestDraft is an optional callback for capability-driven test generation.
	// If nil, a draft is built from the behavior spec in state.
	GenerateTestDraft func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.DraftContent, error)
}

func (p *TestDraftPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	var content interaction.DraftContent
	if p.GenerateTestDraft != nil {
		var err error
		content, err = p.GenerateTestDraft(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		content = defaultTestDraft(mc)
	}

	actions := []interaction.ActionSlot{
		{ID: "write", Label: "Write tests", Shortcut: "y", Kind: interaction.ActionConfirm, Default: true},
	}
	for _, item := range content.Items {
		if item.Editable {
			actions = append(actions, interaction.ActionSlot{
				ID:    fmt.Sprintf("edit_%s", item.ID),
				Label: fmt.Sprintf("Edit %s", item.ID),
				Kind:  interaction.ActionFreetext,
			})
		}
	}
	if content.Addable {
		actions = append(actions, interaction.ActionSlot{
			ID:    "add",
			Label: "Add test case",
			Kind:  interaction.ActionFreetext,
		})
	}

	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameDraft,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: content,
		Actions: actions,
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}

	if err := mc.Emitter.Emit(ctx, frame); err != nil {
		return interaction.PhaseOutcome{}, err
	}

	resp, err := mc.Emitter.AwaitResponse(ctx)
	if err != nil {
		return interaction.PhaseOutcome{}, err
	}

	updates := map[string]any{
		"test_draft.response": resp.ActionID,
		"test_draft.items":    content.Items,
	}
	if resp.Text != "" {
		updates["test_draft.edit_text"] = resp.Text
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

func defaultTestDraft(mc interaction.PhaseMachineContext) interaction.DraftContent {
	items := []interaction.DraftItem{}

	if spec, ok := mc.State["specify.spec"].(BehaviorSpec); ok {
		for i, c := range spec.AllCases() {
			items = append(items, interaction.DraftItem{
				ID:        fmt.Sprintf("test-%d", i+1),
				Content:   c.Description,
				Editable:  true,
				Removable: true,
			})
		}
	}

	if len(items) == 0 {
		items = []interaction.DraftItem{
			{ID: "test-1", Content: "Test basic functionality", Editable: true, Removable: true},
		}
	}

	return interaction.DraftContent{
		Kind:    "test_list",
		Items:   items,
		Addable: true,
	}
}

// TestResultPhase presents initial test run results. In TDD, "all red" is
// expected and NOT rendered as failure — the action label is "Implement".
type TestResultPhase struct {
	// RunTests is an optional callback for running the generated tests.
	// If nil, a placeholder "all_red" result is returned.
	RunTests func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.ResultContent, error)
}

func (p *TestResultPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	// Emit status.
	statusFrame := interaction.InteractionFrame{
		Kind:  interaction.FrameStatus,
		Mode:  mc.Mode,
		Phase: mc.Phase,
		Content: interaction.StatusContent{
			Message: "Running tests...",
			Phase:   mc.Phase,
		},
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}
	if err := mc.Emitter.Emit(ctx, statusFrame); err != nil {
		return interaction.PhaseOutcome{}, err
	}

	var result interaction.ResultContent
	if p.RunTests != nil {
		var err error
		result, err = p.RunTests(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		result = tddRedResultFromState(mc.State)
	}

	// In TDD, "all_red" is normal — label is "Implement", not "Fix".
	actions := []interaction.ActionSlot{
		{ID: "implement", Label: "Implement", Shortcut: "y", Kind: interaction.ActionConfirm, Default: true},
		{ID: "add_tests", Label: "Add more tests", Kind: interaction.ActionConfirm, TargetPhase: "specify"},
		{ID: "fix_test", Label: "Fix a test", Kind: interaction.ActionFreetext},
	}

	resultFrame := interaction.InteractionFrame{
		Kind:    interaction.FrameResult,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: result,
		Actions: actions,
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}
	if err := mc.Emitter.Emit(ctx, resultFrame); err != nil {
		return interaction.PhaseOutcome{}, err
	}

	resp, err := mc.Emitter.AwaitResponse(ctx)
	if err != nil {
		return interaction.PhaseOutcome{}, err
	}

	updates := map[string]any{
		"review_tests.response": resp.ActionID,
		"review_tests.result":   result,
		"tdd.phase":             "red",
	}

	switch resp.ActionID {
	case "add_tests":
		return interaction.PhaseOutcome{
			Advance:      true,
			JumpTo:       "specify",
			StateUpdates: updates,
		}, nil
	case "fix_test":
		updates["review_tests.fix_text"] = resp.Text
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

// GreenStatusPhase presents per-test pass/fail status after implementation.
type GreenStatusPhase struct {
	// RunTests is an optional callback for running the tests after implementation.
	// If nil, a placeholder "passed" result is returned.
	RunTests func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.ResultContent, error)
}

func (p *GreenStatusPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	// Emit status.
	statusFrame := interaction.InteractionFrame{
		Kind:  interaction.FrameStatus,
		Mode:  mc.Mode,
		Phase: mc.Phase,
		Content: interaction.StatusContent{
			Message: "Running tests after implementation...",
			Phase:   mc.Phase,
		},
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}
	if err := mc.Emitter.Emit(ctx, statusFrame); err != nil {
		return interaction.PhaseOutcome{}, err
	}

	var result interaction.ResultContent
	if p.RunTests != nil {
		var err error
		result, err = p.RunTests(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		result = tddGreenResultFromState(mc.State)
	}

	actions := buildGreenActions(mc.State, result)

	resultFrame := interaction.InteractionFrame{
		Kind:    interaction.FrameResult,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: result,
		Actions: actions,
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}
	if err := mc.Emitter.Emit(ctx, resultFrame); err != nil {
		return interaction.PhaseOutcome{}, err
	}

	resp, err := mc.Emitter.AwaitResponse(ctx)
	if err != nil {
		return interaction.PhaseOutcome{}, err
	}

	updates := map[string]any{
		"green.response": resp.ActionID,
		"green.result":   result,
		"tdd.phase":      "green",
	}

	switch resp.ActionID {
	case "fix":
		return interaction.PhaseOutcome{
			Advance:      true,
			JumpTo:       "implement",
			StateUpdates: updates,
		}, nil
	case "add_tests":
		return interaction.PhaseOutcome{
			Advance:      true,
			JumpTo:       "specify",
			StateUpdates: updates,
		}, nil
	case "refactor":
		updates["green.refactor_constraint"] = "tests must stay green"
		updates["tdd.refactor_requested"] = true
		return interaction.PhaseOutcome{
			Advance:      true,
			JumpTo:       "implement",
			StateUpdates: updates,
		}, nil
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

func buildGreenActions(state map[string]any, result interaction.ResultContent) []interaction.ActionSlot {
	refactorRequested, _ := state["tdd.refactor_requested"].(bool)
	if result.Status == "passed" {
		actions := []interaction.ActionSlot{
			{ID: "done", Label: "Done", Shortcut: "y", Kind: interaction.ActionConfirm, Default: true},
			{ID: "add_tests", Label: "Add more tests", Kind: interaction.ActionConfirm, TargetPhase: "specify"},
		}
		if !refactorRequested {
			actions = append(actions, interaction.ActionSlot{ID: "refactor", Label: "Refactor", Kind: interaction.ActionConfirm})
		}
		return actions
	}

	// Some tests still failing.
	actions := []interaction.ActionSlot{
		{ID: "fix", Label: "Fix failing tests", Kind: interaction.ActionConfirm, Default: true},
		{ID: "add_tests", Label: "Add more tests", Kind: interaction.ActionConfirm, TargetPhase: "specify"},
		{ID: "done", Label: "Accept partial", Kind: interaction.ActionConfirm},
	}
	if !refactorRequested {
		actions = append(actions, interaction.ActionSlot{ID: "refactor", Label: "Refactor", Kind: interaction.ActionConfirm})
	}
	return actions
}

func tddRedResultFromState(state map[string]any) interaction.ResultContent {
	payload := tddStateRecord(state, "euclo.tdd.red_evidence")
	if len(payload) == 0 {
		return interaction.ResultContent{
			Status: "all_red",
			Evidence: []interaction.EvidenceItem{
				{Kind: "test_correlation", Detail: "All tests fail as expected (TDD red phase)"},
			},
		}
	}
	status := strings.TrimSpace(strings.ToLower(fmt.Sprint(payload["status"])))
	result := interaction.ResultContent{
		Status: redResultStatus(status),
	}
	result.Evidence = append(result.Evidence, verificationEvidenceItems(payload)...)
	if summary := strings.TrimSpace(fmt.Sprint(payload["summary"])); summary != "" && len(result.Evidence) == 0 {
		result.Evidence = append(result.Evidence, interaction.EvidenceItem{Kind: "verification", Detail: summary})
	}
	return result
}

func tddGreenResultFromState(state map[string]any) interaction.ResultContent {
	payload := tddStateRecord(state, "euclo.tdd.green_evidence")
	if len(payload) == 0 {
		payload = tddStateRecord(state, "euclo.tdd.refactor_evidence")
	}
	if len(payload) == 0 {
		return interaction.ResultContent{Status: "passed"}
	}
	status := strings.TrimSpace(strings.ToLower(fmt.Sprint(payload["status"])))
	result := interaction.ResultContent{
		Status: greenResultStatus(status),
	}
	result.Evidence = append(result.Evidence, verificationEvidenceItems(payload)...)
	if summary := strings.TrimSpace(fmt.Sprint(payload["summary"])); summary != "" && len(result.Evidence) == 0 {
		result.Evidence = append(result.Evidence, interaction.EvidenceItem{Kind: "verification", Detail: summary})
	}
	return result
}

func tddStateRecord(state map[string]any, key string) map[string]any {
	if state == nil {
		return nil
	}
	raw, ok := state[key]
	if !ok || raw == nil {
		return nil
	}
	record, _ := raw.(map[string]any)
	return record
}

func verificationEvidenceItems(payload map[string]any) []interaction.EvidenceItem {
	rawChecks, ok := payload["checks"]
	if !ok || rawChecks == nil {
		return nil
	}
	items := []interaction.EvidenceItem{}
	switch typed := rawChecks.(type) {
	case []any:
		for _, item := range typed {
			record, _ := item.(map[string]any)
			if len(record) == 0 {
				continue
			}
			detail := strings.TrimSpace(fmt.Sprint(record["details"]))
			if detail == "" {
				detail = strings.TrimSpace(fmt.Sprint(record["name"]))
			}
			items = append(items, interaction.EvidenceItem{
				Kind:     "verification",
				Detail:   firstNonEmpty(detail, strings.TrimSpace(fmt.Sprint(record["status"]))),
				Location: strings.TrimSpace(fmt.Sprint(record["working_directory"])),
			})
		}
	case []map[string]any:
		for _, record := range typed {
			detail := strings.TrimSpace(fmt.Sprint(record["details"]))
			if detail == "" {
				detail = strings.TrimSpace(fmt.Sprint(record["name"]))
			}
			items = append(items, interaction.EvidenceItem{
				Kind:     "verification",
				Detail:   firstNonEmpty(detail, strings.TrimSpace(fmt.Sprint(record["status"]))),
				Location: strings.TrimSpace(fmt.Sprint(record["working_directory"])),
			})
		}
	}
	return items
}

func redResultStatus(status string) string {
	switch status {
	case "fail", "failed":
		return "all_red"
	case "pass", "passed":
		return "passed"
	default:
		return "all_red"
	}
}

func greenResultStatus(status string) string {
	switch status {
	case "pass", "passed":
		return "passed"
	case "fail", "failed":
		return "failed"
	default:
		return "partial"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
