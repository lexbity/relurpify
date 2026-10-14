package pipeline

import (
	"context"
	"errors"
	"sync"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/model"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// validContract returns contract metadata accepted by ContractDescriptor.Validate.
func validContract() ContractDescriptor {
	return ContractDescriptor{
		Name:        "fake.contract",
		Description: "contract for tests",
		Metadata: ContractMetadata{
			InputKey:      "input",
			OutputKey:     "output",
			SchemaVersion: "1.0.0",
		},
	}
}

// fakeStage is a scriptable Stage: each hook returns the programmed value and
// records the calls it received.
type fakeStage struct {
	name             string
	contract         ContractDescriptor
	prompt           string
	promptErr        error
	decoded          any
	decodeErr        error
	validateErr      error
	validateErrs     []error // consumed per Validate call; falls back to validateErr
	applyErr         error
	allowTools       bool
	allowedToolNames []string
	requiresTool     bool

	applyCalls []any
	mu         sync.Mutex
}

func newFakeStage(name string) *fakeStage {
	return &fakeStage{
		name:     name,
		contract: validContract(),
		prompt:   "prompt for " + name,
		decoded:  map[string]any{"ok": true},
	}
}

func (s *fakeStage) Name() string { return s.name }

func (s *fakeStage) Contract() ContractDescriptor {
	c := s.contract
	c.Metadata.AllowTools = s.allowTools
	return c
}

func (s *fakeStage) BuildPrompt(_ *contextdata.Envelope) (string, error) {
	return s.prompt, s.promptErr
}

func (s *fakeStage) Decode(_ *model.LLMResponse) (any, error) {
	return s.decoded, s.decodeErr
}

func (s *fakeStage) Validate(_ any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.validateErrs) > 0 {
		err := s.validateErrs[0]
		s.validateErrs = s.validateErrs[1:]
		return err
	}
	return s.validateErr
}

func (s *fakeStage) Apply(_ *contextdata.Envelope, output any) error {
	s.mu.Lock()
	s.applyCalls = append(s.applyCalls, output)
	s.mu.Unlock()
	return s.applyErr
}

// AllowedToolNames implements ToolScopedStage.
func (s *fakeStage) AllowedToolNames() []string { return s.allowedToolNames }

// RequiresToolExecution implements ToolRequiredStage.
func (s *fakeStage) RequiresToolExecution(_ *execution.Task, _ *contextdata.Envelope, _ []ports.Tool) bool {
	return s.requiresTool
}

// fakeModel is a scriptable model.LanguageModel.
type fakeModel struct {
	generateResp *model.LLMResponse
	generateErr  error
	chatResp     *model.LLMResponse
	chatErr      error

	generateCalls int
	chatCalls     int
	prompts       []string
	mu            sync.Mutex
}

func (m *fakeModel) Generate(_ context.Context, prompt string, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.mu.Lock()
	m.generateCalls++
	m.prompts = append(m.prompts, prompt)
	m.mu.Unlock()
	if m.generateErr != nil {
		return nil, m.generateErr
	}
	if m.generateResp != nil {
		return m.generateResp, nil
	}
	return &model.LLMResponse{Text: "ok"}, nil
}

func (m *fakeModel) GenerateStream(_ context.Context, _ string, _ *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *fakeModel) Chat(_ context.Context, _ []model.Message, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return m.chatResponse()
}

func (m *fakeModel) ChatWithTools(_ context.Context, _ []model.Message, _ []model.LLMToolSpec, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.mu.Lock()
	m.chatCalls++
	m.mu.Unlock()
	return m.chatResponse()
}

func (m *fakeModel) chatResponse() (*model.LLMResponse, error) {
	if m.chatErr != nil {
		return nil, m.chatErr
	}
	if m.chatResp != nil {
		return m.chatResp, nil
	}
	return &model.LLMResponse{Text: "chat ok"}, nil
}

// recordingTelemetry captures emitted events for assertions.
type recordingTelemetry struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (r *recordingTelemetry) Emit(ev telemetry.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *recordingTelemetry) emitted() []telemetry.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]telemetry.Event(nil), r.events...)
}

var errBoom = errors.New("boom")
