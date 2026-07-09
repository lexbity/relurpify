package agenttest

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/app/envcomposition"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

type fakeAgentExecutor struct {
	callCount int
	failCount int   // succeed after this many failures
	failWith  error // error to return when failing
}

func (f *fakeAgentExecutor) Execute(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*execution.Result, error) {
	f.callCount++
	if f.callCount <= f.failCount {
		return nil, f.failWith
	}
	return &execution.Result{Success: true}, nil
}

func (f *fakeAgentExecutor) Initialize(config *execution.Config) error { return nil }

func (f *fakeAgentExecutor) Capabilities() []string { return nil }

func (f *fakeAgentExecutor) BuildGraph(ctx context.Context, task *execution.Task) (*agentgraph.Graph, error) {
	return nil, nil
}

type fakeManagedBackend struct {
	resetCount int
}

func (f *fakeManagedBackend) Model() llm.LanguageModel                          { return nil }
func (f *fakeManagedBackend) Embedder() llm.Embedder                            { return nil }
func (f *fakeManagedBackend) Capabilities() llm.BackendCapabilities             { return llm.BackendCapabilities{} }
func (f *fakeManagedBackend) ModelContextSize(ctx context.Context) (int, error) { return 0, nil }
func (f *fakeManagedBackend) Health(ctx context.Context) (*llm.HealthReport, error) {
	return &llm.HealthReport{State: "ready"}, nil
}
func (f *fakeManagedBackend) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	return nil, nil
}
func (f *fakeManagedBackend) Warm(ctx context.Context) error       { return nil }
func (f *fakeManagedBackend) Close() error                         { return nil }
func (f *fakeManagedBackend) SetDebugLogging(enabled bool)         {}
func (f *fakeManagedBackend) SetProfile(profile *llm.ModelProfile) {}
func (f *fakeManagedBackend) Reset(ctx context.Context, strategy string) error {
	f.resetCount++
	return nil
}

func TestExecuteWithRetryNoRetryOnSuccess(t *testing.T) {
	exec := &PreparedRunExecutor{}
	fake := &fakeAgentExecutor{failCount: 0}
	exec.WithAgentOverride(fake)

	desc := &PreparedRunDescriptor{RunID: "r1", Instruction: "do it", MaxRetries: 2}
	task := &execution.Task{ID: "r1", Instruction: "do it"}
	env := contextdata.NewEnvelope("r1", "r1")

	result, attempts, triggeredBy, err := exec.executeWithRetry(context.Background(), desc, task, env, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if len(triggeredBy) != 0 {
		t.Fatalf("triggeredBy = %v, want empty", triggeredBy)
	}
}

func TestExecuteWithRetryRespectsNoneStrategy(t *testing.T) {
	exec := &PreparedRunExecutor{}
	fake := &fakeAgentExecutor{failCount: 1, failWith: fmt.Errorf("execution failed")}
	exec.WithAgentOverride(fake)

	desc := &PreparedRunDescriptor{RunID: "r1", Instruction: "do it", MaxRetries: 2, BackendResetStrategy: "none"}
	task := &execution.Task{ID: "r1", Instruction: "do it"}
	env := contextdata.NewEnvelope("r1", "r1")

	result, attempts, triggeredBy, err := exec.executeWithRetry(context.Background(), desc, task, env, io.Discard)
	if err == nil {
		t.Fatal("expected error")
	}
	if result != nil {
		t.Fatal("expected nil result on failure")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if len(triggeredBy) != 0 {
		t.Fatalf("triggeredBy = %v, want empty", triggeredBy)
	}
}

func TestExecuteWithRetryRetriesOnModelError(t *testing.T) {
	exec := &PreparedRunExecutor{}
	fake := &fakeAgentExecutor{failCount: 1, failWith: fmt.Errorf("model failure")}
	exec.WithAgentOverride(fake)
	exec.model = &envcomposition.ModelRuntime{
		Backend: &fakeManagedBackend{},
	}

	desc := &PreparedRunDescriptor{RunID: "r1", Instruction: "do it", MaxRetries: 2, BackendResetStrategy: "model"}
	task := &execution.Task{ID: "r1", Instruction: "do it"}
	env := contextdata.NewEnvelope("r1", "r1")

	result, attempts, triggeredBy, err := exec.executeWithRetry(context.Background(), desc, task, env, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success after retry")
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if len(triggeredBy) != 1 {
		t.Fatalf("triggeredBy = %v, want 1 entry", triggeredBy)
	}
}

func TestExecuteWithRetryExhaustsMaxRetries(t *testing.T) {
	exec := &PreparedRunExecutor{}
	fake := &fakeAgentExecutor{failCount: 99, failWith: fmt.Errorf("persistent failure")}
	exec.WithAgentOverride(fake)
	exec.model = &envcomposition.ModelRuntime{
		Backend: &fakeManagedBackend{},
	}

	desc := &PreparedRunDescriptor{RunID: "r1", Instruction: "do it", MaxRetries: 1, BackendResetStrategy: "model"}
	task := &execution.Task{ID: "r1", Instruction: "do it"}
	env := contextdata.NewEnvelope("r1", "r1")

	result, attempts, triggeredBy, err := exec.executeWithRetry(context.Background(), desc, task, env, io.Discard)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if result != nil {
		t.Fatal("expected nil result on exhaustion")
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2 (1 initial + 1 retry with MaxRetries=1)", attempts)
	}
	if len(triggeredBy) != 1 {
		t.Fatalf("triggeredBy = %v, want 1 entry (only one retry attempted)", triggeredBy)
	}
}

func TestResetBackendModelCallsBackendReset(t *testing.T) {
	fakeBk := &fakeManagedBackend{}
	exec := &PreparedRunExecutor{
		model: &envcomposition.ModelRuntime{Backend: fakeBk},
	}
	desc := &PreparedRunDescriptor{BackendResetStrategy: "model"}

	if err := exec.resetBackend(context.Background(), desc); err != nil {
		t.Fatalf("resetBackend: %v", err)
	}
	if fakeBk.resetCount != 1 {
		t.Fatalf("resetCount = %d, want 1", fakeBk.resetCount)
	}
}

func TestResetBackendServerRequiresService(t *testing.T) {
	exec := &PreparedRunExecutor{}
	desc := &PreparedRunDescriptor{BackendResetStrategy: "server", BackendService: ""}

	err := exec.resetBackend(context.Background(), desc)
	if err == nil {
		t.Fatal("expected error for server reset without backend_service")
	}
	if !strings.Contains(err.Error(), "server reset requires backend_service") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResetBackendNoneIsNoop(t *testing.T) {
	exec := &PreparedRunExecutor{}
	desc := &PreparedRunDescriptor{BackendResetStrategy: "none"}

	if err := exec.resetBackend(context.Background(), desc); err != nil {
		t.Fatalf("resetBackend: %v", err)
	}
}

func TestClassifyFailureInfra(t *testing.T) {
	if classifyFailure(fmt.Errorf("timeout")) != "infra" {
		t.Fatal("timeout should be infra")
	}
	if classifyFailure(fmt.Errorf("deadline exceeded")) != "infra" {
		t.Fatal("deadline should be infra")
	}
}

func TestClassifyFailureSecurity(t *testing.T) {
	if classifyFailure(fmt.Errorf("permission denied")) != "security" {
		t.Fatal("permission denied should be security")
	}
	if classifyFailure(fmt.Errorf("access denied to resource")) != "security" {
		t.Fatal("access denied should be security")
	}
}

func TestClassifyFailureAssertion(t *testing.T) {
	if classifyFailure(fmt.Errorf("unexpected output")) != "assertion" {
		t.Fatal("unexpected error should be assertion")
	}
	if classifyFailure(fmt.Errorf("verification failed")) != "assertion" {
		t.Fatal("verification failure should be assertion")
	}
}

func TestInferTaskTypeCodeModification(t *testing.T) {
	desc := &PreparedRunDescriptor{
		Verification: PreparedVerificationContract{
			Steps: []PreparedVerificationStep{{Tool: "go_test"}},
		},
	}
	if inferTaskType(desc) != "code_modification" {
		t.Fatalf("expected code_modification, got %q", inferTaskType(desc))
	}
}

func TestInferTaskTypeChat(t *testing.T) {
	desc := &PreparedRunDescriptor{}
	if inferTaskType(desc) != "chat" {
		t.Fatalf("expected chat, got %q", inferTaskType(desc))
	}
}
