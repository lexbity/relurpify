package tui

import (
	"strings"
	"testing"

	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestStructuredResultFromCoreUsesCapabilityEnvelope(t *testing.T) {
	toolResult := &core.ToolResult{
		Success:  true,
		Data:     map[string]any{"summary": "planned successfully"},
		Metadata: map[string]any{},
	}
	envelope := core.NewCapabilityResultEnvelope(core.CapabilityDescriptor{
		ID:         "relurpic:planner.plan",
		Name:       "planner.plan",
		TrustClass: core.TrustClassBuiltinTrusted,
		EffectClasses: []core.EffectClass{
			core.EffectClassContextInsertion,
		},
	}, toolResult, core.ContentDispositionSummarized, nil, &core.ApprovalBinding{
		CapabilityID:   "relurpic:planner.plan",
		CapabilityName: "planner.plan",
		WorkflowID:     "wf-1",
		TaskID:         "task-1",
	})
	toolResult.Metadata["capability_result_envelope"] = envelope

	result := structuredResultFromCore(&core.Result{
		NodeID:  "planner-node",
		Success: true,
		Data: map[string]any{
			"result": toolResult,
		},
	})

	if result == nil {
		t.Fatalf("expected structured result")
	}
	if result.NodeID != "planner-node" {
		t.Fatalf("expected node id planner-node, got %q", result.NodeID)
	}
	if result.Envelope == nil {
		t.Fatalf("expected envelope")
	}
	if result.Envelope.CapabilityName != "planner.plan" {
		t.Fatalf("expected planner.plan envelope, got %q", result.Envelope.CapabilityName)
	}
	if result.Envelope.Insertion.Action != string(core.InsertionActionDirect) {
		t.Fatalf("expected direct insertion, got %q", result.Envelope.Insertion.Action)
	}
	if len(result.Envelope.Blocks) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Envelope.Blocks))
	}
	if result.Envelope.Blocks[0].Type != "text" {
		t.Fatalf("expected text block, got %q", result.Envelope.Blocks[0].Type)
	}
	if !strings.Contains(result.Envelope.Blocks[0].Body, "planned successfully") {
		t.Fatalf("expected summary text in block body, got %q", result.Envelope.Blocks[0].Body)
	}
}

func TestRenderMessageIncludesStructuredResultDetails(t *testing.T) {
	msg := Message{
		ID:   "msg-1",
		Role: RoleAgent,
		Content: MessageContent{
			Text: "Primary answer",
			Result: &StructuredResult{
				NodeID:  "architect-node",
				Success: true,
				Envelope: &StructuredResultEnvelope{
					CapabilityID:   "relurpic:architect.execute",
					CapabilityName: "architect.execute",
					TrustClass:     "builtin-trusted",
					Disposition:    "raw",
					Insertion: StructuredInsertion{
						Action: "summarized",
						Reason: "remote output summarized",
					},
					Approval: &StructuredApprovalBinding{
						WorkflowID:    "wf-1",
						TaskID:        "task-2",
						EffectClasses: []string{"write_workspace"},
					},
					Blocks: []StructuredContentBlock{{
						Type:    "structured",
						Summary: "structured output",
						Body:    `{"summary":"implemented"}`,
						Provenance: map[string]string{
							"capability": "relurpic:architect.execute",
							"trust":      "builtin-trusted",
						},
					}},
				},
			},
			Expanded: map[string]bool{},
		},
	}

	rendered := RenderMessage(msg, 100, "")
	for _, want := range []string{
		"🧾 Result",
		"capability=architect.execute",
		"insertion=summarized",
		"approval:",
		`"summary": "implemented"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected rendered message to contain %q, got:\n%s", want, rendered)
		}
	}
}

func TestNotificationBarViewIncludesApprovalDetails(t *testing.T) {
	req := &fauthorization.PermissionRequest{
		ID:            "hitl-structured",
		Permission:    core.PermissionDescriptor{Action: "provider:connect", Resource: "mcp://peer", Metadata: map[string]string{"approval_kind": "provider_operation", "provider_id": "mcp-client"}},
		Justification: "connect remote peer",
		Risk:          fauthorization.RiskLevelHigh,
	}

	queue := &NotificationQueue{}
	queue.PushHITL(req)

	bar := NewNotificationBar(queue)
	view := bar.View()
	for _, want := range []string{
		"provider_operation approval: provider:connect -> mcp-client",
		"kind=provider_operation",
		"risk=high",
		"target=mcp-client",
		"why=connect remote peer",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected notification view to contain %q, got:\n%s", want, view)
		}
	}
}
