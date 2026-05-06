package testsuite

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	"codeburg.org/lexbit/relurpify/named/euclo/policy"
)

func TestDryRunEndToEndAmbiguousInteractionAndHITL(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "mixed.go", "package demo\n")

	caps := newCapabilityRegistry(t, "euclo:cap.targeted_refactor")
	classifier := &mockTier2Classifier{
		responses: map[string]tier2Response{
			"implementation": {Sequence: []string{"euclo:cap.targeted_refactor"}, Operator: "OR"},
		},
	}
	broker := authorization.NewHITLBroker(5 * time.Second)
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(caps)),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithCapabilityClassifier(classifier),
		orchestrate.WithHITLBroker(broker),
	)

	env := contextdata.NewEnvelope("task-interaction", "session-interaction")
	seedTask(env, "implement and review the handler", "mixed.go")
	env.SetWorkingValue("euclo.policy_decision", &policy.PolicyDecision{
		MutationPermitted:    false,
		HITLRequired:         true,
		VerificationRequired: false,
		ReasonCodes:          []string{"approval_required"},
	}, contextdata.MemoryClassTask)
	runPreIngestion(t, env, dir, []string{"mixed.go"})

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
					ApprovedBy: "testsuite",
					Scope:      authorization.GrantScopeOneTime,
				})
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	if err := graph.Execute(context.Background(), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}
	<-done

	if !mustBoolValue(t, env, "euclo.interaction.frame_requested") {
		t.Fatal("expected interaction frame to be requested")
	}
	if !mustBoolValue(t, env, "euclo.hitl_triggered") {
		t.Fatal("expected HITL to be triggered")
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected execution to complete after HITL approval")
	}
	if classifier.callCount() == 0 {
		t.Fatal("expected tier-2 classifier to be invoked")
	}
}
