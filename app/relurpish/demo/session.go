// Package demo supplies explicitly selected, stateful application scenarios to
// the production frontend. It has no filesystem, process, network, environment,
// authorization, or live runtime dependencies. Restarting discards all changes.
//
// Simulated contract inventory:
//   - Submit/Next: model admission, recipe execution, and tool outcomes.
//   - Answer: authored questions and permission decisions (no real grants).
//   - Cancel: cancellation and retained terminal outcomes.
//   - BuildDoctorReport/InitializeWorkspaceFromTemplates: diagnosis and setup.
//   - List/Open/Save/TriggerRecipe and RecipeCatalog: source artifacts and prompts.
//   - InspectConfiguration: scenario configuration and actual strategy metadata.
//   - Services/ControlServices: scoped lifecycle state and partial restart failure.
//
// Real execution ownership, actual-call provenance, and atomic initialization
// remain integration work. Scenarios exercise the same host reducers, command
// bar, focus, circuit, inspector, and Doctor as live mode.
package demo

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

// Clock schedules application events; frontend animation never decides results.
type Clock interface {
	After(time.Duration) <-chan time.Time
}
type wallClock struct{}

func (wallClock) After(duration time.Duration) <-chan time.Time { return time.After(duration) }

// Scenarios returns the available explicit starting states.
func Scenarios() []string {
	return []string{"quiet", "execution", "question", "approval", "expired", "failure", "first-launch", "doctor-ready", "doctor-degraded", "doctor-blocked", "services-partial"}
}

type Session struct {
	mu          sync.Mutex
	clock       Clock
	scenario    string
	changed     chan struct{}
	state       tui.FrontendSnapshot
	phase       int
	run         uint64
	initialized bool
	recipes     map[string]tui.RecipeFile
	services    []tui.ServiceInfo
}

func New(scenario string, clock Clock) (*Session, error) {
	if !slices.Contains(Scenarios(), scenario) {
		return nil, fmt.Errorf("unknown demo scenario %q; choose one of %v", scenario, Scenarios())
	}
	if clock == nil {
		clock = wallClock{}
	}
	s := &Session{clock: clock, scenario: scenario, changed: make(chan struct{}, 1), initialized: scenario != "first-launch"}
	if err := s.loadRecipes(); err != nil {
		return nil, err
	}
	s.services = []tui.ServiceInfo{
		{ID: "knowledge.bootstrap", Status: tui.ServiceStatusRunning, Owner: "DEMO workspace", Source: "DEMO in memory"},
		{ID: "knowledge.index", Status: tui.ServiceStatusRunning, Owner: "DEMO workspace", Source: "DEMO in memory"},
		{ID: "telemetry.flush", Status: tui.ServiceStatusRunning, Owner: "DEMO workspace", Source: "DEMO in memory"},
	}
	s.state = tui.FrontendSnapshot{Revision: 1, Status: "DEMO · ready · type a request or open /run"}
	if scenario == "first-launch" {
		s.state.Status = "DEMO · configuration missing · /doctor"
	}
	if slices.Contains([]string{"execution", "failure", "question", "approval", "expired"}, scenario) {
		s.beginLocked("Repair slugify and verify its tests")
		if slices.Contains([]string{"question", "approval", "expired"}, scenario) {
			s.openDecisionLocked()
		}
	}
	return s, nil
}

func (s *Session) Current() tui.FrontendSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clone(s.state)
}

// DEMO ONLY: Simulates model admission and the selected recipe for these scenarios.
// LIVE CONTRACT: ConversationSession.Submit, runtime-owned request admission.
// REPLACEMENT: Supply real admission/results through this port; retain equivalent interaction coverage.
// SIDE EFFECTS: Demo state only; no files, services, model calls, or grants.
func (s *Session) Submit(ctx context.Context, text string) (tui.FrontendSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return tui.FrontendSnapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initialized {
		return clone(s.state), errors.New("demo workspace needs initialization")
	}
	if s.state.Running {
		return clone(s.state), errors.New("a demo request is already running")
	}
	if text == "" || len(text) > 65536 {
		return clone(s.state), errors.New("request must contain 1–65536 bytes")
	}
	s.beginLocked(text)
	s.publishLocked()
	return clone(s.state), nil
}

func (s *Session) beginLocked(text string) {
	s.run++
	s.phase = 0
	s.state.Running = true
	s.state.Decision = nil
	s.state.Messages = append(s.state.Messages, tui.ConversationLine{Role: "user", Text: text})
	s.state.Circuit = tui.CircuitSnapshot{RecipeID: "demo.repair", Name: "Repair and verify", Route: "debug", Notice: "DEMO ONLY: illustrative step and context observations; no live execution occurred.", Steps: []tui.CircuitStep{
		{ID: "understand", Strategy: "react", Goal: "Read the failing behavior and identify the smallest repair", Status: "active", ToolScopes: []string{"file_read", "file_search"}},
		{ID: "repair", Strategy: "react", Goal: "Repair slugify", Dependencies: []string{"understand"}, Gate: "one-time approval", ToolScopes: []string{"file_write"}},
		{ID: "verify", Strategy: "react", Goal: "Run focused tests and inspect the result", Dependencies: []string{"repair"}, ToolScopes: []string{"python_pytest"}},
	}}
	s.state.Status = "DEMO · understand active · /run inspects execution"
	s.state.Revision++
}

// DEMO ONLY: Advances scheduled recipe/tool outcomes, including failure/expiry.
// LIVE CONTRACT: ConversationSession.Next, execution observation subscription.
// REPLACEMENT: Deliver real immutable snapshots/cursors; keep these scenarios for UI regression coverage.
// SIDE EFFECTS: Demo state only; no files, services, model calls, or grants.
func (s *Session) Next(ctx context.Context) (tui.FrontendSnapshot, error) {
	s.mu.Lock()
	scheduled := s.state.Running && (s.state.Decision == nil || s.scenario == "expired")
	s.mu.Unlock()
	var tick <-chan time.Time
	if scheduled {
		tick = s.clock.After(1400 * time.Millisecond)
	}
	select {
	case <-ctx.Done():
		return tui.FrontendSnapshot{}, ctx.Err()
	case <-s.changed:
		return s.Current(), nil
	case <-tick:
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.state.Running {
			if s.state.Decision != nil && s.scenario == "expired" {
				s.state.Decision = nil
				s.state.Running = false
				s.state.Status = "DEMO · decision expired; no answer was applied"
			} else if s.state.Decision == nil {
				s.advanceLocked()
			}
			s.state.Revision++
		}
		return clone(s.state), nil
	}
}

func (s *Session) advanceLocked() {
	steps := s.state.Circuit.Steps
	if s.phase >= len(steps) {
		return
	}
	steps[s.phase].Status = "done"
	steps[s.phase].DurationMS = 1400
	s.phase++
	if s.phase == 1 && s.phase < len(steps) && slices.Contains([]string{"question", "approval"}, s.scenario) {
		s.openDecisionLocked()
		return
	}
	if s.phase == 2 && s.scenario == "failure" {
		steps[1].Status = "failed"
		steps[1].Error = "Simulated sandbox write failed: workspace became read-only"
		s.state.Running = false
		s.state.Status = "DEMO · repair failed · /inspect repair"
		s.state.Messages = append(s.state.Messages, tui.ConversationLine{Role: "agent", Text: "DEMO: Repair failed. Verification was not run; no successful test result is claimed."})
		return
	}
	if s.phase == len(steps) {
		s.state.Running = false
		s.state.Status = "DEMO · completed · /run retains the observed result"
		s.state.Messages = append(s.state.Messages, tui.ConversationLine{Role: "agent", Text: "DEMO: The simulated repair and verification completed. No workspace files were modified and no tests actually ran."})
		return
	}
	steps[s.phase].Status = "active"
	s.state.Status = "DEMO · " + steps[s.phase].ID + " active · /run"
}

func (s *Session) openDecisionLocked() {
	kind, question := "question", "Which behavior should slugify preserve?"
	choices := []tui.DecisionChoice{{ID: "ascii", Label: "ASCII letters and numbers"}, {ID: "unicode", Label: "Unicode letters and numbers"}}
	text := true
	if s.scenario == "approval" || s.scenario == "expired" {
		kind, question = "permission", "Allow the simulated repair to write stringkit.py once?"
		choices = []tui.DecisionChoice{{ID: "once", Label: "Allow this operation once"}, {ID: "deny", Label: "Deny this operation"}}
		text = false
	}
	s.state.Decision = &tui.FrontendDecision{ID: fmt.Sprintf("decision-%d", s.run), Kind: kind, Question: question, Choices: choices, AllowText: text}
	s.state.Status = "DEMO · waiting for a deliberate human answer"
}

// DEMO ONLY: Resolves a question/permission choice in the scenario state.
// LIVE CONTRACT: ConversationSession.Answer, authoritative decision resolver.
// REPLACEMENT: Supply real decision CAS/expiry results; retain typing/focus regression scenarios.
// SIDE EFFECTS: Demo state only; no files, services, model calls, or grants.
func (s *Session) Answer(ctx context.Context, id, choice, text string) (tui.FrontendSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return tui.FrontendSnapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	decision := s.state.Decision
	if decision == nil || decision.ID != id {
		return clone(s.state), errors.New("decision expired or already resolved")
	}
	valid := text != "" && decision.AllowText
	for _, item := range decision.Choices {
		valid = valid || choice == item.ID
	}
	if !valid {
		return clone(s.state), errors.New("choose an offered answer or use the separate answer field")
	}
	s.state.Decision = nil
	if choice == "deny" {
		s.state.Running = false
		s.state.Status = "DEMO · operation denied; no permission was granted"
	} else {
		s.state.Status = "DEMO · answer recorded; continuing"
		// Do not ask the same scheduled decision again after it was answered.
		if s.phase == 0 {
			s.state.Circuit.Steps[0].Status = "done"
			s.phase = 1
			s.state.Circuit.Steps[1].Status = "active"
		}
	}
	s.state.Revision++
	s.publishLocked()
	return clone(s.state), nil
}

// DEMO ONLY: Cancels the scenario, retaining its observed partial state.
// LIVE CONTRACT: ConversationSession.Cancel, runtime cancellation receipt/result.
// REPLACEMENT: Return the real owner's cancellation outcome; retain the late-event scenario test.
// SIDE EFFECTS: Demo state only; no files, services, model calls, or grants.
func (s *Session) Cancel(ctx context.Context) (tui.FrontendSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return tui.FrontendSnapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.state.Running {
		return clone(s.state), errors.New("no demo request is running")
	}
	s.state.Running = false
	s.state.Decision = nil
	s.state.Status = "DEMO · canceled · partial observations retained"
	s.state.Revision++
	s.publishLocked()
	return clone(s.state), nil
}

// DEMO ONLY: Supplies scenario diagnosis; no dependency is actually probed.
// LIVE CONTRACT: DoctorReader.BuildDoctorReport.
// REPLACEMENT: Supply the real diagnostic service; keep ready/degraded/blocked frontend coverage.
// SIDE EFFECTS: Demo state only; no files, services, model calls, or grants.
func (s *Session) BuildDoctorReport(ctx context.Context) runtimesvc.DoctorReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := runtimesvc.DoctorReport{Workspace: "DEMO workspace (memory only)", ConfigRoot: "DEMO/relurpify_cfg", WorkspacePresent: s.initialized, ConfigExists: s.initialized, StarterTemplatesReady: true, SandboxReady: s.initialized, ModelReady: s.initialized, CheckedAt: time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)}
	r.Inference = runtimesvc.InferenceBackendReport{Provider: "DEMO", SelectedModel: "simulated model", State: llm.BackendHealthReady}
	r.Dependencies = []runtimesvc.DependencyStatus{{Name: "sandbox", Available: s.initialized, Blocking: !s.initialized, Details: "DEMO: sandbox availability is simulated; no process was launched"}}
	if s.scenario == "doctor-degraded" {
		r.Dependencies = append(r.Dependencies, runtimesvc.DependencyStatus{Name: "knowledge index", Degraded: true, Details: "DEMO: stale optional index; execution remains available"})
	}
	if s.scenario == "doctor-blocked" || ctx.Err() != nil {
		r.ModelReady = false
		r.Inference.State = llm.BackendHealthUnhealthy
		r.Inference.Error = "DEMO: model unavailable"
	}
	return r
}

// DEMO ONLY: Materializes a simulated workspace configuration in memory.
// LIVE CONTRACT: DoctorOperations.InitializeWorkspaceFromTemplates.
// REPLACEMENT: Supply reviewed real materialization results; retain create/cancel/preserve UI scenarios.
// SIDE EFFECTS: Demo state only; no files, services, model calls, or grants.
func (s *Session) InitializeWorkspaceFromTemplates(overwrite bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if overwrite {
		return errors.New("demo initialization preserves existing files; overwrite is not offered")
	}
	s.initialized = true
	s.state.Status = "DEMO · workspace initialized in memory; no files changed"
	s.state.Revision++
	s.publishLocked()
	return nil
}

func (s *Session) publishLocked() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}
func clone(value tui.FrontendSnapshot) tui.FrontendSnapshot {
	value.Messages = slices.Clone(value.Messages)
	value.Circuit.Steps = slices.Clone(value.Circuit.Steps)
	for i := range value.Circuit.Steps {
		value.Circuit.Steps[i].Dependencies = slices.Clone(value.Circuit.Steps[i].Dependencies)
		value.Circuit.Steps[i].ToolScopes = slices.Clone(value.Circuit.Steps[i].ToolScopes)
	}
	if value.Decision != nil {
		copy := *value.Decision
		copy.Choices = slices.Clone(copy.Choices)
		value.Decision = &copy
	}
	return value
}
