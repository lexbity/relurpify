package euclo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/jobs"
	eucloingestion "codeburg.org/lexbit/relurpify/named/euclo/ingestion"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	"codeburg.org/lexbit/relurpify/named/euclo/policy"
	recipepkg "codeburg.org/lexbit/relurpify/named/euclo/recipes"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type recordingTelemetry struct {
	mu     sync.Mutex
	events []core.Event
}

func (r *recordingTelemetry) Emit(event core.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recordingTelemetry) types() []core.EventType {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]core.EventType, 0, len(r.events))
	for _, event := range r.events {
		out = append(out, event.Type)
	}
	return out
}

type testCapabilityHandler struct {
	descriptor core.CapabilityDescriptor
	invoke     func(context.Context, *contextdata.Envelope, map[string]any) (*contracts.CapabilityExecutionResult, error)
}

func (h *testCapabilityHandler) Descriptor(context.Context, *contextdata.Envelope) core.CapabilityDescriptor {
	return h.descriptor
}

func (h *testCapabilityHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*contracts.CapabilityExecutionResult, error) {
	if h != nil && h.invoke != nil {
		return h.invoke(ctx, env, args)
	}
	return &contracts.CapabilityExecutionResult{
		Success: true,
		Data: map[string]any{
			"capability_id": h.descriptor.ID,
		},
	}, nil
}

type testSubmitter struct {
	job *jobs.Job
}

func (s *testSubmitter) Submit(context.Context, jobs.JobSpec) (*jobs.Job, error) {
	if s != nil && s.job != nil {
		job := *s.job
		return &job, nil
	}
	return &jobs.Job{
		ID:        "job-1",
		Spec:      jobs.JobSpec{},
		State:     jobs.JobStateCompleted,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}, nil
}

func newIntegrationCapabilityRegistry(t *testing.T, ids ...string) *capability.CapabilityRegistry {
	t.Helper()
	reg := capability.NewCapabilityRegistry()
	for _, id := range ids {
		handler := &testCapabilityHandler{
			descriptor: core.CapabilityDescriptor{
				ID:            id,
				Name:          id,
				Kind:          core.CapabilityKindTool,
				RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
				Availability:  core.AvailabilitySpec{Available: true},
			},
			invoke: func(id string) func(context.Context, *contextdata.Envelope, map[string]any) (*contracts.CapabilityExecutionResult, error) {
				return func(context.Context, *contextdata.Envelope, map[string]any) (*contracts.CapabilityExecutionResult, error) {
					return &contracts.CapabilityExecutionResult{
						Success: true,
						Data: map[string]any{
							"capability_id": id,
							"result":        id + ":ok",
						},
					}, nil
				}
			}(id),
		}
		if err := reg.RegisterInvocableCapability(handler); err != nil {
			t.Fatalf("register capability %s: %v", id, err)
		}
	}
	return reg
}

func newRecipeRegistry(t *testing.T, recipe *recipepkg.ThoughtRecipe) *recipepkg.RecipeRegistry {
	t.Helper()
	reg := recipepkg.NewRecipeRegistry()
	if err := reg.Register(recipe); err != nil {
		t.Fatalf("register recipe: %v", err)
	}
	return reg
}

func workspaceEnv(reg *capability.CapabilityRegistry) agentenv.WorkspaceEnvironment {
	return agentenv.WorkspaceEnvironment{Registry: reg}
}

func seedTask(env *contextdata.Envelope, instruction string) *core.Task {
	task := &core.Task{
		ID:          env.TaskID,
		Type:        "euclo",
		Instruction: instruction,
		Data:        map[string]any{},
		Context:     map[string]any{},
		Metadata:    map[string]any{},
	}
	seedTaskEnvelope(env, task)
	return task
}

func runRootGraph(t *testing.T, graph *orchestrate.RootGraph, env *contextdata.Envelope, telemetry *recordingTelemetry) {
	t.Helper()
	ctx := core.WithTelemetry(context.Background(), telemetry)
	if err := graph.Execute(ctx, env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}
}

func assertEventSubsequence(t *testing.T, got []core.EventType, want []core.EventType) {
	t.Helper()
	pos := 0
	for _, eventType := range got {
		if pos >= len(want) {
			break
		}
		if eventType == want[pos] {
			pos++
		}
	}
	if pos != len(want) {
		t.Fatalf("event subsequence not found: got=%v want=%v", got, want)
	}
}

func mustStringValue(t *testing.T, env *contextdata.Envelope, key string) string {
	t.Helper()
	value, ok := env.GetWorkingValue(key)
	if !ok {
		t.Fatalf("missing envelope value %q", key)
	}
	s, ok := value.(string)
	if !ok {
		t.Fatalf("envelope value %q is %T, want string", key, value)
	}
	return s
}

func mustBoolValue(t *testing.T, env *contextdata.Envelope, key string) bool {
	t.Helper()
	value, ok := env.GetWorkingValue(key)
	if !ok {
		t.Fatalf("missing envelope value %q", key)
	}
	b, ok := value.(bool)
	if !ok {
		t.Fatalf("envelope value %q is %T, want bool", key, value)
	}
	return b
}

func TestIntegrationImplementationPath(t *testing.T) {
	reg := newIntegrationCapabilityRegistry(t, "euclo:cap.targeted_refactor")
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(reg)),
		orchestrate.WithCapabilityRegistry(reg),
	)

	env := contextdata.NewEnvelope("task-impl", "session-impl")
	seedTask(env, "add a cache to the handler")
	telemetry := &recordingTelemetry{}
	runRootGraph(t, graph, env, telemetry)

	if got := mustStringValue(t, env, "euclo.execution.kind"); got != "capability" {
		t.Fatalf("execution kind = %q, want capability", got)
	}
	if got := mustStringValue(t, env, "euclo.route.kind"); got != "capability" {
		t.Fatalf("route kind = %q, want capability", got)
	}
	if got := mustStringValue(t, env, "euclo.route.capability_id"); got != "euclo:cap.targeted_refactor" {
		t.Fatalf("route capability id = %q, want euclo:cap.targeted_refactor", got)
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected execution.completed to be true")
	}

	assertEventSubsequence(t, telemetry.types(), []core.EventType{
		core.EventType("euclo.route.selected"),
		core.EventType("euclo.route.completed"),
		core.EventType("euclo.execution.complete"),
	})
}

func TestIntegrationReviewPathAndRecipeCarryForward(t *testing.T) {
	caps := newIntegrationCapabilityRegistry(t, "euclo:cap.code_review")
	recipe := &recipepkg.ThoughtRecipe{
		ID:         "euclo.recipe.default",
		APIVersion: "euclo/v1",
		Metadata: recipepkg.RecipeMetadata{
			Name: "integration-review",
		},
		Sequence: recipepkg.RecipeSequence{
			Steps: []recipepkg.RecipeStep{
				{
					ID:           "step-1",
					CapabilityID: "euclo:cap.capture",
					Captures: map[string]string{
						"output": "first_output",
					},
				},
				{
					ID:           "step-2",
					CapabilityID: "euclo:cap.consume",
					Bindings: map[string]string{
						"input": "first_output",
					},
					Captures: map[string]string{
						"result": "second_output",
					},
				},
			},
		},
	}
	recipes := newRecipeRegistry(t, recipe)
	if err := caps.RegisterInvocableCapability(&testCapabilityHandler{
		descriptor: core.CapabilityDescriptor{
			ID:            "euclo:cap.capture",
			Name:          "euclo:cap.capture",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
			Availability:  core.AvailabilitySpec{Available: true},
		},
		invoke: func(context.Context, *contextdata.Envelope, map[string]any) (*contracts.CapabilityExecutionResult, error) {
			return &contracts.CapabilityExecutionResult{
				Success: true,
				Data: map[string]any{
					"output": "alpha",
				},
			}, nil
		},
	}); err != nil {
		t.Fatalf("register capture capability: %v", err)
	}
	if err := caps.RegisterInvocableCapability(&testCapabilityHandler{
		descriptor: core.CapabilityDescriptor{
			ID:            "euclo:cap.consume",
			Name:          "euclo:cap.consume",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
			Availability:  core.AvailabilitySpec{Available: true},
		},
		invoke: func(context.Context, *contextdata.Envelope, map[string]any) (*contracts.CapabilityExecutionResult, error) {
			return &contracts.CapabilityExecutionResult{
				Success: true,
				Data: map[string]any{
					"result": "omega",
				},
			}, nil
		},
	}); err != nil {
		t.Fatalf("register consume capability: %v", err)
	}
	node := orchestrate.NewRecipeExecutorNode("euclo.execute_recipe").
		WithWorkspaceEnvironment(workspaceEnv(caps)).
		WithRecipeRegistry(recipes)

	env := contextdata.NewEnvelope("task-review", "session-review")
	seedTask(env, "review the auth package")
	env.SetWorkingValue("euclo.route_selection", &orchestrate.RouteSelection{
		RouteKind: "recipe",
		RecipeID:  "euclo.recipe.default",
	}, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.route.recipe_id", "euclo.recipe.default", contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful recipe execution, got %#v", result)
	}

	if got := mustStringValue(t, env, "euclo.execution.kind"); got != "recipe" {
		t.Fatalf("execution kind = %q, want recipe", got)
	}
	captured, ok := env.GetWorkingValue("first_output")
	if !ok {
		t.Fatal("missing first_output capture")
	}
	capturedMap, ok := captured.(map[string]any)
	if !ok {
		t.Fatalf("first_output = %T, want map[string]any", captured)
	}
	if got := capturedMap["output"]; got != "alpha" {
		t.Fatalf("first_output.output = %v, want alpha", got)
	}
	if !mustBoolValue(t, env, "euclo.recipe.step.step-2.success") {
		t.Fatal("expected step-2 to succeed")
	}
}

func TestIntegrationDebugAndMigrationVerification(t *testing.T) {
	caps := newIntegrationCapabilityRegistry(t, "euclo:cap.bisect", "euclo:cap.symbol_trace", "euclo:cap.api_compat")
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(caps)),
		orchestrate.WithCapabilityRegistry(caps),
	)

	t.Run("debug", func(t *testing.T) {
		env := contextdata.NewEnvelope("task-debug", "session-debug")
		seedTask(env, "fix the panic in startup.go")
		runRootGraph(t, graph, env, &recordingTelemetry{})

		if got := mustStringValue(t, env, "euclo.execution.kind"); got != "capability" {
			t.Fatalf("execution kind = %q, want capability", got)
		}
		if got := mustStringValue(t, env, "euclo.route.capability_id"); got != "euclo:cap.bisect" {
			t.Fatalf("route capability id = %q, want euclo:cap.bisect", got)
		}
		if cls, ok := env.GetWorkingValue("euclo.intent_classification"); !ok {
			t.Fatal("missing intent classification")
		} else if intent, ok := cls.(*intake.IntentClassification); !ok || intent == nil || !intent.RequiresVerification {
			t.Fatalf("expected debug classification to require verification, got %#v", cls)
		}
		if seq, ok := env.GetWorkingValue("euclo.capability_sequence"); !ok {
			t.Fatal("missing capability sequence")
		} else if got := seq.([]string); len(got) < 2 || got[0] != "euclo:cap.bisect" || got[1] != "euclo:cap.symbol_trace" {
			t.Fatalf("unexpected debug capability sequence: %#v", seq)
		}
	})

	t.Run("migration", func(t *testing.T) {
		env := contextdata.NewEnvelope("task-migration", "session-migration")
		seedTask(env, "migrate to the new API")
		runRootGraph(t, graph, env, &recordingTelemetry{})

		if got := mustStringValue(t, env, "euclo.execution.kind"); got != "capability" {
			t.Fatalf("execution kind = %q, want capability", got)
		}
		if got := mustStringValue(t, env, "euclo.route.capability_id"); got != "euclo:cap.api_compat" {
			t.Fatalf("route capability id = %q, want euclo:cap.api_compat", got)
		}
		if cls, ok := env.GetWorkingValue("euclo.intent_classification"); !ok {
			t.Fatal("missing intent classification")
		} else if intent, ok := cls.(*intake.IntentClassification); !ok || intent == nil || !intent.RequiresVerification {
			t.Fatalf("expected migration classification to require verification, got %#v", cls)
		}
	})
}

func TestIntegrationHITLApprovalPath(t *testing.T) {
	caps := newIntegrationCapabilityRegistry(t, "euclo:cap.targeted_refactor")
	broker := authorization.NewHITLBroker(5 * time.Second)
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(caps)),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithHITLBroker(broker),
	)

	env := contextdata.NewEnvelope("task-hitl", "session-hitl")
	seedTask(env, "add a cache to the handler")
	env.SetWorkingValue("euclo.policy_decision", &policy.PolicyDecision{
		MutationPermitted:    false,
		HITLRequired:         true,
		VerificationRequired: false,
		ReasonCodes:          []string{"approval_required"},
	}, contextdata.MemoryClassTask)

	done := make(chan struct{})
	go func() {
		defer close(done)
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			pending := broker.PendingRequests()
			if len(pending) > 0 {
				_ = broker.Approve(authorization.PermissionDecision{
					RequestID:  pending[0].ID,
					Approved:   true,
					ApprovedBy: "integration-test",
					Scope:      authorization.GrantScopeOneTime,
				})
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	runRootGraph(t, graph, env, &recordingTelemetry{})
	<-done

	if !mustBoolValue(t, env, "euclo.hitl_triggered") {
		t.Fatal("expected HITL to be triggered")
	}
	if resp, ok := env.GetWorkingValue("euclo.hitl_response"); !ok || resp == nil {
		t.Fatal("expected HITL response to be recorded")
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected execution to complete after approval")
	}
}

func TestIntegrationHITLRejectionPath(t *testing.T) {
	gate := policy.NewGateNode("euclo.policy_gate", policy.NewEvaluator())
	broker := authorization.NewHITLBroker(5 * time.Second)
	gate.WithHITLBroker(broker)

	env := contextdata.NewEnvelope("task-reject", "session-reject")
	env.SetWorkingValue("euclo.policy_decision", &policy.PolicyDecision{
		MutationPermitted:    false,
		HITLRequired:         true,
		VerificationRequired: false,
		ReasonCodes:          []string{"approval_required"},
	}, contextdata.MemoryClassTask)

	done := make(chan struct{})
	go func() {
		defer close(done)
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			pending := broker.PendingRequests()
			if len(pending) > 0 {
				_ = broker.Deny(pending[0].ID, "integration-test-rejection")
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	_, err := gate.Execute(context.Background(), env)
	<-done
	if err == nil {
		t.Fatal("expected HITL rejection to return an error")
	}
	if got := mustStringValue(t, env, "euclo.outcome.category"); got != "hitl_rejected" {
		t.Fatalf("outcome category = %q, want hitl_rejected", got)
	}
	if got := mustStringValue(t, env, "euclo.outcome.reason"); !strings.Contains(got, "integration-test-rejection") {
		t.Fatalf("outcome reason = %q, want rejection reason", got)
	}
}

func TestIntegrationSessionResumePreservesRoute(t *testing.T) {
	caps := newIntegrationCapabilityRegistry(t, "euclo:cap.code_review")
	agent := New(workspaceEnv(caps), WithConfig(DefaultConfig()))
	agent.initialized = true
	agent.recipeRegistry = newRecipeRegistry(t, &recipepkg.ThoughtRecipe{
		APIVersion: "euclo/v1",
		Metadata:   recipepkg.RecipeMetadata{Name: "resume"},
		Sequence: recipepkg.RecipeSequence{
			Steps: []recipepkg.RecipeStep{{ID: "step-1", CapabilityID: "euclo:cap.code_review"}},
		},
	})
	agent.resumeClassification = &intake.IntentClassification{WinningFamily: "review"}
	agent.resumeRouteSelection = &orchestrate.RouteSelection{RouteKind: "recipe", RecipeID: "euclo.recipe.default"}

	graph, err := agent.BuildGraph(&core.Task{ID: "task-resume", Instruction: "resume"})
	if err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}
	if got := graph.StartNodeID(); got != "euclo.policy_gate" {
		t.Fatalf("start node = %q, want euclo.policy_gate", got)
	}
}

func TestIntegrationBackgroundJobPath(t *testing.T) {
	telemetry := &recordingTelemetry{}
	submitter := &testSubmitter{
		job: &jobs.Job{
			ID:    "job-123",
			State: jobs.JobStateCompleted,
			Spec: jobs.JobSpec{
				Kind:  "euclo.background",
				Queue: "background",
			},
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
	}
	node := orchestrate.NewBackgroundJobNode("euclo.background").
		WithSubmitter(submitter).
		WithTelemetry(reporting.NewEucloTelemetry(telemetry))

	env := contextdata.NewEnvelope("task-bg", "session-bg")
	env.SetWorkingValue("task.input", &core.Task{ID: "task-bg", Instruction: "enqueue background work"}, contextdata.MemoryClassTask)

	result, err := node.Execute(core.WithTelemetry(context.Background(), telemetry), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful job submission, got %#v", result)
	}
	if got := mustStringValue(t, env, "euclo.background.job_id"); got != "job-123" {
		t.Fatalf("job id = %q, want job-123", got)
	}
	if !mustBoolValue(t, env, "euclo.background.job_completed") {
		t.Fatal("expected background job to be marked completed")
	}
	assertEventSubsequence(t, telemetry.types(), []core.EventType{
		core.EventType("euclo.job.submitted"),
		core.EventType("euclo.job.completed"),
	})
}

func TestIntegrationUserFilesIngested(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "handler.go")
	if err := os.WriteFile(path, []byte("package demo\n\nfunc Handler() {}\n"), 0600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	node := eucloingestion.NewIngestionNode("euclo.ingest", eucloingestion.IngestionSpec{
		Mode:          eucloingestion.IngestionModeFilesOnly,
		WorkspaceRoot: dir,
	})
	env := contextdata.NewEnvelope("task-ingest", "session-ingest")
	env.SetWorkingValue("euclo.task.envelope", &intake.TaskEnvelope{
		TaskID:    "task-ingest",
		SessionID: "session-ingest",
		UserFiles: []string{"handler.go"},
	}, contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful ingestion, got %#v", result)
	}
	ingestionResult, ok := env.GetWorkingValue("euclo.ingestion_result")
	if !ok || ingestionResult == nil {
		t.Fatal("expected ingestion result in envelope")
	}
	summary, ok := env.GetWorkingValue("euclo.ingestion.summary")
	if !ok || summary == nil {
		t.Fatal("expected ingestion summary in envelope")
	}
	if got := summary.(map[string]any)["user_files_ingested"]; got != 1 {
		t.Fatalf("user_files_ingested = %v, want 1", got)
	}
}

func TestIntegrationWorkspaceIncremental(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workspace.go")
	if err := os.WriteFile(path, []byte("package demo\n"), 0600); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
	}
	run("init")
	run("config", "user.email", "integration@example.com")
	run("config", "user.name", "Integration Test")
	run("add", "workspace.go")
	run("commit", "-m", "initial")
	if err := os.WriteFile(path, []byte("package demo\n\nvar Changed = true\n"), 0600); err != nil {
		t.Fatalf("update workspace file: %v", err)
	}

	node := eucloingestion.NewIngestionNode("euclo.ingest", eucloingestion.IngestionSpec{
		Mode:          eucloingestion.IngestionModeIncremental,
		WorkspaceRoot: dir,
		SinceRef:      "HEAD~1",
	})
	env := contextdata.NewEnvelope("task-incremental", "session-incremental")
	env.SetWorkingValue("euclo.task.envelope", &intake.TaskEnvelope{
		TaskID:    "task-incremental",
		SessionID: "session-incremental",
	}, contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful incremental ingestion, got %#v", result)
	}
	ingestionResult, ok := env.GetWorkingValue("euclo.ingestion_result")
	if !ok || ingestionResult == nil {
		t.Fatal("expected ingestion result in envelope")
	}
	if got := ingestionResult.(*eucloingestion.IngestionResult).Mode; got != eucloingestion.IngestionModeIncremental {
		t.Fatalf("mode = %q, want incremental", got)
	}
}

func TestIntegrationRouteFallbackRecovery(t *testing.T) {
	reg := capability.NewCapabilityRegistry()
	primary := &testCapabilityHandler{
		descriptor: core.CapabilityDescriptor{
			ID:            "euclo:cap.targeted_refactor",
			Name:          "euclo:cap.targeted_refactor",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
			Availability:  core.AvailabilitySpec{Available: false, Reason: "tool not enabled"},
		},
	}
	fallback := &testCapabilityHandler{
		descriptor: core.CapabilityDescriptor{
			ID:            "euclo:cap.rename_symbol",
			Name:          "euclo:cap.rename_symbol",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
			Availability:  core.AvailabilitySpec{Available: true},
		},
	}
	if err := reg.RegisterInvocableCapability(primary); err != nil {
		t.Fatalf("register primary: %v", err)
	}
	if err := reg.RegisterInvocableCapability(fallback); err != nil {
		t.Fatalf("register fallback: %v", err)
	}

	result, err := orchestrate.Dispatch(
		context.Background(),
		contextdata.NewEnvelope("task-route", "session-route"),
		orchestrate.RouteRequest{
			CapabilityID: "euclo:cap.targeted_refactor",
			FallbackID:   "euclo:cap.rename_symbol",
		},
		reg,
		nil,
	)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected route result")
	}
	if !result.FallbackTaken {
		t.Fatal("expected fallback to be taken")
	}
	if result.RouteID != "euclo:cap.rename_symbol" {
		t.Fatalf("route id = %q, want fallback capability", result.RouteID)
	}
}

func TestIntegrationTelemetryStreamComplete(t *testing.T) {
	reg := newIntegrationCapabilityRegistry(t, "euclo:cap.targeted_refactor")
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(reg)),
		orchestrate.WithCapabilityRegistry(reg),
	)
	env := contextdata.NewEnvelope("task-telemetry", "session-telemetry")
	seedTask(env, "add a cache to the handler")
	telemetry := &recordingTelemetry{}
	runRootGraph(t, graph, env, telemetry)

	assertEventSubsequence(t, telemetry.types(), []core.EventType{
		core.EventType("euclo.route.selected"),
		core.EventType("euclo.route.completed"),
		core.EventType("euclo.execution.complete"),
	})
}
