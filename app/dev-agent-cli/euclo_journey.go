package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

type EucloJourneyFrameRecord struct {
	StepIndex int            `json:"step_index"`
	Kind      string         `json:"kind"`
	Mode      string         `json:"mode"`
	Phase     string         `json:"phase,omitempty"`
	Timestamp string         `json:"timestamp"`
	Content   map[string]any `json:"content,omitempty"`
	Actions   []string       `json:"actions,omitempty"`
}

type EucloJourneyResponseRecord struct {
	StepIndex  int      `json:"step_index"`
	Mode       string   `json:"mode"`
	Phase      string   `json:"phase,omitempty"`
	ActionID   string   `json:"action_id,omitempty"`
	Text       string   `json:"text,omitempty"`
	Selections []string `json:"selections,omitempty"`
}

type EucloJourneyTransitionRecord struct {
	StepIndex int    `json:"step_index"`
	FromMode  string `json:"from_mode"`
	ToMode    string `json:"to_mode"`
	Trigger   string `json:"trigger,omitempty"`
	Accepted  bool   `json:"accepted"`
	Reason    string `json:"reason,omitempty"`
}

type EucloJourneyTranscriptEntry struct {
	StepIndex      int    `json:"step_index"`
	Kind           string `json:"kind"`
	Mode           string `json:"mode"`
	Phase          string `json:"phase,omitempty"`
	Message        string `json:"message,omitempty"`
	DurationMillis int64  `json:"duration_millis,omitempty"`
}

type journeyExecutionState struct {
	mode             string
	phase            string
	lastFrameKind    string
	lastFrameActions []string
	lastResponse     interaction.UserResponse
	lastTrigger      *TriggerCatalogEntry
}

func executeEucloJourneyScript(ctx context.Context, script EucloJourneyScript, catalog *EucloCatalog) (*EucloJourneyReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if catalog == nil {
		catalog = newEucloCatalog()
	}
	if strings.TrimSpace(script.ScriptVersion) == "" {
		script.ScriptVersion = "v1alpha1"
	}
	runMode := strings.ToLower(strings.TrimSpace(script.RecordingMode))
	if runMode == "" {
		runMode = "live"
	}

	report := &EucloJourneyReport{
		RunClass:              "journey",
		Workspace:             ensureWorkspace(),
		RunMode:               runMode,
		ScriptVersion:         script.ScriptVersion,
		InitialMode:           strings.TrimSpace(script.InitialMode),
		FinalMode:             strings.TrimSpace(script.InitialMode),
		RecordingMode:         strings.TrimSpace(script.RecordingMode),
		ExpectedTerminalState: cloneStringMap(script.ExpectedTerminalState),
		TerminalState:         cloneStringMap(script.InitialContext),
	}
	if report.TerminalState == nil {
		report.TerminalState = map[string]any{}
	}

	state := journeyExecutionState{
		mode:  report.InitialMode,
		phase: "",
	}
	if state.mode == "" {
		state.mode = "chat"
		report.InitialMode = state.mode
		report.FinalMode = state.mode
	}
	report.TerminalState["current_mode"] = state.mode
	if len(script.InitialContext) > 0 {
		for k, v := range script.InitialContext {
			report.TerminalState[k] = v
		}
	}

	var failures []string
	for idx, step := range script.Steps {
		start := time.Now().UTC()
		prevMode := state.mode
		stepReport := EucloJourneyStepReport{
			Index:      idx,
			Kind:       strings.TrimSpace(step.Kind),
			Mode:       strings.TrimSpace(step.Mode),
			Phase:      strings.TrimSpace(step.Phase),
			Trigger:    strings.TrimSpace(step.Trigger),
			Capability: strings.TrimSpace(step.Capability),
			Success:    true,
			Updated:    map[string]any{},
		}

		frame, response, responseRecord, err := executeJourneyStep(ctx, catalog, &state, report.TerminalState, step, idx)
		if frame != nil {
			stepReport.FrameKind = string(frame.Kind)
			stepReport.Mode = frame.Mode
			stepReport.Phase = frame.Phase
			report.Frames = append(report.Frames, journeyFrameRecordForStep(idx, frame))
		}
		if responseRecord != nil {
			stepReport.ResponseAction = responseRecord.ActionID
			report.Responses = append(report.Responses, *responseRecord)
		}
		if err != nil {
			stepReport.Success = false
			stepReport.Message = err.Error()
			failures = append(failures, err.Error())
		} else {
			stepReport.Message = "step applied"
		}
		if response.ActionID != "" {
			stepReport.ResponseAction = response.ActionID
		}

		if stepReport.FrameKind == "" && frame != nil {
			stepReport.FrameKind = string(frame.Kind)
		}
		if stepReport.Kind == "trigger.fire" && stepReport.ResponseAction == "" {
			stepReport.ResponseAction = response.ActionID
		}
		stepReport.Artifacts = artifactStrings(report.TerminalState["euclo.artifacts"])

		report.Transcript = append(report.Transcript, EucloJourneyTranscriptEntry{
			StepIndex:      idx,
			Kind:           stepReport.Kind,
			Mode:           safeFrameMode(frame),
			Phase:          safeFramePhase(frame),
			Message:        stepReport.Message,
			DurationMillis: time.Since(start).Milliseconds(),
		})

		stepReport.DurationMillis = time.Since(start).Milliseconds()

		if step.ExpectedMode != "" && !strings.EqualFold(stepReport.Mode, step.ExpectedMode) {
			failures = append(failures, fmt.Sprintf("steps[%d].expected_mode mismatch: got %s want %s", idx, stepReport.Mode, step.ExpectedMode))
			stepReport.Success = false
		}
		if step.ExpectedPhase != "" && !strings.EqualFold(stepReport.Phase, step.ExpectedPhase) {
			failures = append(failures, fmt.Sprintf("steps[%d].expected_phase mismatch: got %s want %s", idx, stepReport.Phase, step.ExpectedPhase))
			stepReport.Success = false
		}
		if step.ExpectedFrameKind != "" && !strings.EqualFold(stepReport.FrameKind, step.ExpectedFrameKind) {
			failures = append(failures, fmt.Sprintf("steps[%d].expected_frame_kind mismatch: got %s want %s", idx, stepReport.FrameKind, step.ExpectedFrameKind))
			stepReport.Success = false
		}
		if step.ExpectedResponseAction != "" && !strings.EqualFold(stepReport.ResponseAction, step.ExpectedResponseAction) {
			failures = append(failures, fmt.Sprintf("steps[%d].expected_response_action mismatch: got %s want %s", idx, stepReport.ResponseAction, step.ExpectedResponseAction))
			stepReport.Success = false
		}
		if step.ExpectedArtifactKind != "" && !containsString(stepReport.Artifacts, step.ExpectedArtifactKind) {
			failures = append(failures, fmt.Sprintf("steps[%d].expected_artifact_kind %q not observed", idx, step.ExpectedArtifactKind))
			stepReport.Success = false
		}
		for _, key := range step.ExpectedStateKeys {
			if _, ok := report.TerminalState[key]; !ok {
				failures = append(failures, fmt.Sprintf("steps[%d].expected_state_key %q not present", idx, key))
				stepReport.Success = false
			}
		}

		if frame != nil && frame.Kind == interaction.FrameTransition && response.ActionID != "" {
			report.Transitions = append(report.Transitions, EucloJourneyTransitionRecord{
				StepIndex: idx,
				FromMode:  prevMode,
				ToMode:    firstNonEmpty(step.Mode, state.mode),
				Trigger:   step.Trigger,
				Accepted:  response.ActionID != "reject",
				Reason:    transitionReasonForStep(step, response),
			})
		}
		report.Steps = append(report.Steps, stepReport)
	}

	if len(script.ExpectedTerminalState) > 0 {
		for key, expected := range script.ExpectedTerminalState {
			if !equalValue(report.TerminalState[key], expected) {
				failures = append(failures, fmt.Sprintf("terminal state %q mismatch: got %v want %v", key, report.TerminalState[key], expected))
			}
		}
	}

	report.Success = len(failures) == 0
	report.Failures = uniqueStrings(failures)
	report.FinalMode = state.mode
	report.CurrentPhase = state.phase
	sort.Slice(report.Frames, func(i, j int) bool { return report.Frames[i].StepIndex < report.Frames[j].StepIndex })
	sort.Slice(report.Responses, func(i, j int) bool { return report.Responses[i].StepIndex < report.Responses[j].StepIndex })
	sort.Slice(report.Transitions, func(i, j int) bool { return report.Transitions[i].StepIndex < report.Transitions[j].StepIndex })
	sort.Slice(report.Transcript, func(i, j int) bool { return report.Transcript[i].StepIndex < report.Transcript[j].StepIndex })
	return report, nil
}

func executeJourneyStep(
	ctx context.Context,
	catalog *EucloCatalog,
	state *journeyExecutionState,
	terminal map[string]any,
	step EucloJourneyStep,
	index int,
) (*interaction.InteractionFrame, interaction.UserResponse, *EucloJourneyResponseRecord, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	emitter := interaction.NewTestFrameEmitter()
	emitter.DefaultActionID = defaultJourneyActionForStep(step)
	frame := buildJourneyFrame(state, step, index)
	if err := emitter.Emit(ctx, frame); err != nil {
		return nil, interaction.UserResponse{}, nil, err
	}
	response, err := emitter.AwaitResponse(ctx)
	if err != nil {
		return &frame, interaction.UserResponse{}, nil, err
	}
	record := &EucloJourneyResponseRecord{
		StepIndex:  index,
		Mode:       frame.Mode,
		Phase:      frame.Phase,
		ActionID:   response.ActionID,
		Text:       response.Text,
		Selections: append([]string(nil), response.Selections...),
	}

	switch step.Kind {
	case "mode.select":
		mode := firstNonEmpty(strings.TrimSpace(step.Mode), stringValue(step.Value), state.mode)
		state.mode = mode
		terminal["current_mode"] = mode
		terminal["journey.mode_selected"] = mode
		if step.ExpectedPhase != "" {
			state.phase = step.ExpectedPhase
		}
	case "submode.select":
		phase := firstNonEmpty(strings.TrimSpace(step.Phase), stringValue(step.Value), state.phase)
		state.phase = phase
		terminal["current_submode"] = phase
	case "trigger.fire":
		text := firstNonEmpty(strings.TrimSpace(step.Text), stringValue(step.Value), step.Trigger)
		trigger, ok := catalog.ResolveTrigger(state.mode, text)
		terminal["last_trigger_text"] = text
		terminal["last_trigger_matched"] = ok
		if ok && trigger != nil {
			terminal["last_trigger"] = trigger
			terminal["euclo.artifacts"] = appendArtifactString(terminal["euclo.artifacts"], triggerArtifactName(trigger))
			if trigger.CapabilityID != "" {
				terminal["selected_capability"] = trigger.CapabilityID
			}
			state.lastTrigger = trigger
			if trigger.PhaseJump != "" {
				state.phase = trigger.PhaseJump
				terminal["current_phase"] = trigger.PhaseJump
			}
			record.ActionID = response.ActionID
			frame.Content = map[string]any{
				"trigger":         trigger,
				"matched":         true,
				"phase_jump":      trigger.PhaseJump,
				"capability_id":   trigger.CapabilityID,
				"selected_layer":  step.ExpectedMode,
				"response_action": response.ActionID,
			}
			frame.Kind = interaction.FrameProposal
		} else {
			frame.Content = map[string]any{
				"trigger":         text,
				"matched":         false,
				"response_action": response.ActionID,
			}
		}
		if trigger != nil {
			record.ActionID = response.ActionID
		}
	case "context.add":
		key := strings.TrimSpace(step.Key)
		if key == "" {
			return &frame, response, record, errors.New("--key is required for context.add")
		}
		terminal[key] = step.Value
		terminal["context.last_updated_key"] = key
		if step.Value != nil {
			terminal["context.last_updated_value"] = step.Value
		}
		state.phase = firstNonEmpty(state.phase, step.ExpectedPhase)
		record.ActionID = response.ActionID
	case "context.remove":
		key := strings.TrimSpace(step.Key)
		if key == "" {
			return &frame, response, record, errors.New("--key is required for context.remove")
		}
		delete(terminal, key)
		terminal["context.last_removed_key"] = key
		record.ActionID = response.ActionID
	case "frame.respond":
		terminal["last_frame_response"] = step.Text
		record.ActionID = response.ActionID
		terminal["journey.frame_response"] = response.Text
	case "transition.accept":
		toMode := firstNonEmpty(strings.TrimSpace(step.Mode), stringValue(step.Value))
		if toMode != "" {
			state.mode = toMode
			terminal["current_mode"] = toMode
		}
		terminal["transition.accepted"] = toMode
		record.ActionID = response.ActionID
	case "transition.reject":
		terminal["transition.rejected"] = firstNonEmpty(strings.TrimSpace(step.Mode), stringValue(step.Value))
		record.ActionID = response.ActionID
	case "hitl.approve":
		terminal["hitl.decision"] = "approve"
		record.ActionID = response.ActionID
	case "hitl.deny":
		terminal["hitl.decision"] = "deny"
		record.ActionID = response.ActionID
	case "workflow.resume":
		terminal["workflow.resumed"] = true
		record.ActionID = response.ActionID
	case "plan.promote":
		terminal["plan.promoted"] = true
		record.ActionID = response.ActionID
	case "artifact.expect":
		if err := assertExpectedArtifact(terminal["euclo.artifacts"], step.Expected); err != nil {
			return &frame, response, record, err
		}
		record.ActionID = response.ActionID
		if step.ExpectedArtifactKind != "" {
			terminal["journey.expected_artifact_kind"] = step.ExpectedArtifactKind
		}
	default:
		return &frame, response, record, fmt.Errorf("unknown journey step kind %q", step.Kind)
	}

	return &frame, response, record, nil
}

func buildJourneyFrame(state *journeyExecutionState, step EucloJourneyStep, index int) interaction.InteractionFrame {
	kind := frameKindForJourneyStep(step.Kind)
	actions := journeyActionsForStep(step.Kind)
	content := map[string]any{
		"step_index": index,
		"kind":       step.Kind,
		"mode":       state.mode,
		"phase":      firstNonEmpty(strings.TrimSpace(step.Phase), state.phase),
		"trigger":    strings.TrimSpace(step.Trigger),
		"capability": strings.TrimSpace(step.Capability),
		"text":       strings.TrimSpace(step.Text),
	}
	return interaction.InteractionFrame{
		Kind:        kind,
		Mode:        state.mode,
		Phase:       firstNonEmpty(strings.TrimSpace(step.Phase), state.phase),
		Content:     content,
		Actions:     actions,
		Continuable: true,
	}
}

func frameKindForJourneyStep(kind string) interaction.FrameKind {
	switch kind {
	case "mode.select", "trigger.fire", "transition.accept", "transition.reject":
		return interaction.FrameTransition
	case "submode.select", "context.add", "context.remove":
		return interaction.FrameDraft
	case "frame.respond":
		return interaction.FrameResult
	case "hitl.approve", "hitl.deny":
		return interaction.FrameQuestion
	case "workflow.resume":
		return interaction.FrameSessionResume
	case "plan.promote":
		return interaction.FrameSummary
	case "artifact.expect":
		return interaction.FrameResult
	default:
		return interaction.FrameStatus
	}
}

func journeyActionsForStep(kind string) []interaction.ActionSlot {
	switch kind {
	case "mode.select", "transition.accept":
		return []interaction.ActionSlot{
			{ID: "accept", Label: "Accept", Kind: interaction.ActionKindPrimary, Default: true},
			{ID: "reject", Label: "Reject", Kind: interaction.ActionKindSecondary},
		}
	case "transition.reject":
		return []interaction.ActionSlot{
			{ID: "reject", Label: "Reject", Kind: interaction.ActionKindPrimary, Default: true},
		}
	case "trigger.fire", "hitl.approve", "hitl.deny", "workflow.resume", "plan.promote", "artifact.expect":
		return []interaction.ActionSlot{
			{ID: "continue", Label: "Continue", Kind: interaction.ActionKindPrimary, Default: true},
		}
	default:
		return []interaction.ActionSlot{
			{ID: "continue", Label: "Continue", Kind: interaction.ActionKindPrimary, Default: true},
		}
	}
}

func defaultJourneyActionForStep(step EucloJourneyStep) string {
	switch step.Kind {
	case "mode.select", "transition.accept":
		return "accept"
	case "transition.reject":
		return "reject"
	case "trigger.fire", "hitl.approve", "hitl.deny", "workflow.resume", "plan.promote", "artifact.expect":
		return "continue"
	default:
		return "continue"
	}
}

func transitionReasonForStep(step EucloJourneyStep, resp interaction.UserResponse) string {
	if strings.TrimSpace(resp.ActionID) != "" {
		return fmt.Sprintf("response=%s", resp.ActionID)
	}
	if strings.TrimSpace(step.Text) != "" {
		return step.Text
	}
	return step.Kind
}

func safeFrameMode(frame *interaction.InteractionFrame) string {
	if frame == nil {
		return ""
	}
	return frame.Mode
}

func safeFramePhase(frame *interaction.InteractionFrame) string {
	if frame == nil {
		return ""
	}
	return frame.Phase
}

func journeyFrameRecordForStep(index int, frame *interaction.InteractionFrame) EucloJourneyFrameRecord {
	if frame == nil {
		return EucloJourneyFrameRecord{StepIndex: index}
	}
	record := EucloJourneyFrameRecord{
		StepIndex: index,
		Kind:      string(frame.Kind),
		Mode:      frame.Mode,
		Phase:     frame.Phase,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if content, ok := frame.Content.(map[string]any); ok {
		record.Content = cloneStringMap(content)
	}
	if len(frame.Actions) > 0 {
		record.Actions = make([]string, 0, len(frame.Actions))
		for _, action := range frame.Actions {
			record.Actions = append(record.Actions, action.ID)
		}
	}
	return record
}
