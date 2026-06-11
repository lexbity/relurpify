package react

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	capresult "codeburg.org/lexbit/relurpify/capability/result"
	capruntime "codeburg.org/lexbit/relurpify/capability/runtime"
	relurpctx "codeburg.org/lexbit/relurpify/context"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/model"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

type reactActNode struct {
	id    string
	agent *ReActAgent
	task  *execution.Task
}

// ID returns the node identifier for the "act" step.
func (n *reactActNode) ID() string { return n.id }

// Type labels the node as a tool execution step.
func (n *reactActNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeTool }

// Contract marks the ReAct act step as a capability-consuming execution node.
func (n *reactActNode) Contract() agentgraph.NodeContract {
	return agentgraph.NodeContract{
		RequiredCapabilities: []agentspec.CapabilitySelector{{
			Kind: agentspec.CapabilityKindTool,
		}},
		SideEffectClass: agentgraph.SideEffectExternal,
		Idempotency:     agentgraph.IdempotencyUnknown,
		ContextPolicy: relurpctx.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "react.decision", "react.tool_calls", "react.*"},
			WriteKeys:                []string{"react.last_tool_result", "react.last_tool_result_*", "react.tool_observations", "react.*"},
			AllowHistoryAccess:       true,
			AllowedMemoryClasses:     []relurpctx.MemoryClass{relurpctx.MemoryClassWorking},
			AllowedDataClasses:       []relurpctx.StateDataClass{relurpctx.StateDataClassTaskMetadata, relurpctx.StateDataClassStepMetadata, relurpctx.StateDataClassArtifactRef, relurpctx.StateDataClassMemoryRef, relurpctx.StateDataClassStructuredState},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 16,
			PreferArtifactReferences: true,
		},
	}
}

// Execute runs any pending tool calls or directly invokes the requested tool
// referenced in the latest decision payload.
func (n *reactActNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	env.SetWorkingValueWithClass("react.execution_phase", "executing", contextdata.MemoryClassTask)
	activeTools := activeToolSet(env)
	if pending, ok := env.GetWorkingValue("react.tool_calls"); ok {
		if calls, ok := pending.([]model.ToolCall); ok && len(calls) > 0 {
			calls = filterToolCalls(calls)
			if len(calls) == 0 {
				env.SetWorkingValueWithClass("react.tool_calls", []model.ToolCall{}, contextdata.MemoryClassTask)
			} else {
				results := make(map[string]any)
				envelopes := make(map[string]*capresult.CapabilityResultEnvelope)
				toolErrors := make([]string, 0)
				overallSuccess := true
				for _, call := range calls {
					callID := call.ID
					if callID == "" {
						callID = NewUUID()
					}
					if !n.capabilityAllowed(call.Name, activeTools) || !n.agent.Tools.HasCapability(call.Name) {
						errResult := &ports.ToolResult{
							Success: false,
							Error:   fmt.Sprintf("tool %q does not exist. Only use tools from the available list.", call.Name),
						}
						envelope := n.capabilityEnvelope(ctx, env, nil, call, errResult)
						n.recordObservation(ctx, env, call, errResult, envelope)
						envelopes[callID] = envelope
						overallSuccess = false
						toolErrors = append(toolErrors, fmt.Sprintf("unknown tool %s", call.Name))
						continue
					}
					if !n.agent.Tools.CapabilityAvailable(ctx, env.State(), call.Name) {
						errResult := &ports.ToolResult{
							Success: false,
							Error:   fmt.Sprintf("tool %q is unavailable right now.", call.Name),
						}
						envelope := n.capabilityEnvelope(ctx, env, nil, call, errResult)
						n.recordObservation(ctx, env, call, errResult, envelope)
						envelopes[callID] = envelope
						overallSuccess = false
						toolErrors = append(toolErrors, fmt.Sprintf("unavailable tool %s", call.Name))
						continue
					}
					n.agent.debugf("%s executing tool=%s id=%s args=%v", n.id, call.Name, callID, call.Args)
					res, err := n.agent.Tools.InvokeCapability(ctx, env.State(), call.Name, call.Args)
					if err != nil {
						// Convert hard tool errors (e.g. schema validation, permission denial)
						// into soft ToolResult failures so the LLM can observe and recover.
						res = &ports.ToolResult{Success: false, Error: err.Error()}
					}
					if res != nil {
						envelope := n.capabilityEnvelope(ctx, env, nil, call, res)
						envelopes[callID] = envelope
						n.recordObservation(ctx, env, call, res, envelope)
						n.latchVerificationSuccess(env, call.Name, res)
						n.refreshIndexesAfterMutation(ctx, call, res)
						results[callID] = map[string]any{
							"success": res.Success,
							"data":    res.Data,
							"error":   res.Error,
						}
						n.agent.debugf("%s tool=%s id=%s result=%v", n.id, call.Name, callID, res.Data)
						if !res.Success {
							overallSuccess = false
							if res.Error != "" {
								toolErrors = append(toolErrors, fmt.Sprintf("%s: %s", call.Name, res.Error))
							} else {
								toolErrors = append(toolErrors, fmt.Sprintf("%s failed", call.Name))
							}
						}
					}
				}
				env.SetWorkingValueWithClass("react.last_tool_result", results, contextdata.MemoryClassTask)
				env.SetWorkingValueWithClass("react.last_tool_result_envelopes", envelopes, contextdata.MemoryClassTask)
				env.SetWorkingValueWithClass("react.tool_calls", []model.ToolCall{}, contextdata.MemoryClassTask)
				result := &execution.Result{
					NodeID:  n.id,
					Success: overallSuccess,
					Data:    execution.NewToolResultPayload(results),
					Metadata: map[string]any{
						"capability_results": envelopes,
					},
				}
				if len(toolErrors) > 0 {
					result.Error = strings.Join(toolErrors, "; ")
				}
				env.SetWorkingValueWithClass("react.last_result", result, contextdata.MemoryClassTask)
				return result, nil
			}
		}
		if n.agent.Config != nil && !n.agent.Config.NativeToolCalling {
			env.SetWorkingValueWithClass("react.tool_calls", []model.ToolCall{}, contextdata.MemoryClassTask)
		}
	}
	val, ok := env.GetWorkingValue("react.decision")
	if !ok {
		return nil, fmt.Errorf("missing decision from think step")
	}
	decision := val.(decisionPayload)
	toolName := strings.TrimSpace(decision.Tool)
	if decision.Complete || toolName == "" || strings.EqualFold(toolName, "none") {
		env.SetWorkingValueWithClass("react.last_tool_result", map[string]any{}, contextdata.MemoryClassTask)
		result := &execution.Result{NodeID: n.id, Success: true}
		env.SetWorkingValueWithClass("react.last_result", result, contextdata.MemoryClassTask)
		return result, nil
	}
	if !n.capabilityAllowed(toolName, activeTools) || !n.agent.Tools.HasCapability(toolName) {
		lower := strings.ToLower(toolName)
		if lower == "" || strings.Contains(lower, "none") {
			env.SetWorkingValueWithClass("react.last_tool_result", map[string]any{}, contextdata.MemoryClassTask)
			result := &execution.Result{NodeID: n.id, Success: true}
			env.SetWorkingValueWithClass("react.last_result", result, contextdata.MemoryClassTask)
			return result, nil
		}
		// Feed error back to the LLM so it can retry with a valid tool name.
		errMsg := fmt.Sprintf("tool %q does not exist. Only use tools from the available list.", toolName)
		env.SetWorkingValueWithClass("react.last_tool_result", map[string]any{"error": errMsg}, contextdata.MemoryClassTask)
		result := &execution.Result{NodeID: n.id, Success: false, Error: errMsg}
		env.SetWorkingValueWithClass("react.last_result", result, contextdata.MemoryClassTask)
		return result, nil
	}
	if !n.agent.Tools.CapabilityAvailable(ctx, env.State(), toolName) {
		errMsg := fmt.Sprintf("tool %q is unavailable right now.", toolName)
		env.SetWorkingValueWithClass("react.last_tool_result", map[string]any{"error": errMsg}, contextdata.MemoryClassTask)
		result := &execution.Result{NodeID: n.id, Success: false, Error: errMsg}
		env.SetWorkingValueWithClass("react.last_result", result, contextdata.MemoryClassTask)
		return result, nil
	}
	res, err := n.agent.Tools.InvokeCapability(ctx, env.State(), toolName, decision.Arguments)
	if err != nil {
		return nil, err
	}
	call := model.ToolCall{
		ID:   NewUUID(),
		Name: decision.Tool,
		Args: decision.Arguments,
	}
	envelope := n.capabilityEnvelope(ctx, env, nil, call, res)
	n.recordObservation(ctx, env, call, res, envelope)
	n.latchVerificationSuccess(env, call.Name, res)
	env.SetWorkingValueWithClass("react.last_tool_result", res.Data, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("react.last_tool_result_envelope", envelope, contextdata.MemoryClassTask)
	n.agent.debugf("%s tool=%s result=%v", n.id, decision.Tool, res.Data)
	result := &execution.Result{
		NodeID:  n.id,
		Success: res.Success,
		Data:    execution.NewToolResultPayload(res.Data),
		Metadata: map[string]any{
			"capability_result": envelope,
		},
		Error: strings.TrimSpace(res.Error),
	}
	n.refreshIndexesAfterMutation(ctx, call, res)
	env.SetWorkingValueWithClass("react.last_result", result, contextdata.MemoryClassTask)
	return result, nil
}

func (n *reactActNode) latchVerificationSuccess(env *contextdata.Envelope, toolName string, res *ports.ToolResult) {
	if env == nil || n == nil || n.agent == nil || n.task == nil || res == nil || !res.Success {
		return
	}
	if !taskNeedsEditing(n.task) || !verificationStopAllowed(n.agent, n.task) {
		return
	}
	// Allow the latch even when no prior file edit was observed — the agent
	// may be verifying already-correct code (verify-only pass, no edits needed).
	if !verificationToolMatches(toolName, n.agent.verificationSuccessTools()) {
		return
	}
	summary := verificationSuccessSummary(toolName, fmt.Sprint(res.Data["stdout"]))
	env.SetWorkingValueWithClass("react.verification_latched_summary", summary, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("react.synthetic_summary", summary, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("react.incomplete_reason", "", contextdata.MemoryClassTask)
}

func (n *reactActNode) capabilityAllowed(name string, active map[string]struct{}) bool {
	if len(active) > 0 {
		if _, ok := active[name]; !ok {
			return false
		}
	}
	return true
}

func (n *reactActNode) capabilityEnvelope(ctx context.Context, env *contextdata.Envelope, tool ports.Tool, call model.ToolCall, res *ports.ToolResult) *capresult.CapabilityResultEnvelope {
	var desc descriptor.CapabilityDescriptor
	if res != nil && res.Metadata != nil {
		if raw, ok := res.Metadata["capability_descriptor"]; ok {
			if typed, ok := raw.(descriptor.CapabilityDescriptor); ok {
				desc = typed
			}
		}
	}
	if desc.ID == "" {
		if n != nil && n.agent != nil {
			if resolved, ok := n.agent.executionCapabilityDescriptor(ctx, call.Name); ok {
				desc = resolved
			}
		}
	}
	if desc.ID == "" {
		if tool != nil {
			desc = descriptor.ToolDescriptor(ctx, tool)
		} else {
			desc = descriptor.CapabilityDescriptor{
				ID:          "tool:" + call.Name,
				Kind:        agentspec.CapabilityKindTool,
				Name:        call.Name,
				Description: call.Name,
				TrustClass:  agentspec.TrustClassWorkspaceTrusted,
				Source: descriptor.CapabilitySource{
					Scope: classification.CapabilityScopeWorkspace,
				},
			}
		}
	}
	var approval *capresult.ApprovalBinding
	if res != nil && res.Metadata != nil {
		if raw, ok := res.Metadata["approval_binding"]; ok {
			if typed, ok := raw.(*capresult.ApprovalBinding); ok {
				approval = typed
			}
		}
	}
	if approval == nil {
		// ApprovalBindingFromCapability already works with envelope WorkingData
		approval = capresult.ApprovalBindingFromCapability(desc, env.WorkingData, call.Args)
	}
	var snapshot *capresult.PolicySnapshot
	if n != nil && n.agent != nil {
		snapshot = n.agent.executionPolicySnapshot(ctx)
	}
	envelope := capresult.NewCapabilityResultEnvelope(desc, res, capresult.ContentDispositionRaw, snapshot, approval)
	if n != nil && n.agent != nil && n.task != nil {
		envelope = capresult.ApplyInsertionDecision(envelope, resolveInsertionDecision(n.agent, n.task, envelope))
	}
	if n != nil && n.agent != nil && n.agent.Config != nil && n.agent.Config.Telemetry != nil {
		metadata := map[string]any{
			"security_event": "insertion_decision",
			"capability_id":  envelope.Descriptor.ID,
			"capability":     envelope.Descriptor.Name,
			"insertion":      string(envelope.Insertion.Action),
		}
		if envelope.Policy != nil {
			metadata["policy_snapshot_id"] = envelope.Policy.ID
		}
		if envelope.Descriptor.Source.ProviderID != "" {
			metadata["provider_id"] = envelope.Descriptor.Source.ProviderID
		}
		if envelope.Descriptor.Source.SessionID != "" {
			metadata["session_id"] = envelope.Descriptor.Source.SessionID
		}
		n.agent.Config.Telemetry.Emit(telemetry.Event{
			Type:      telemetry.EventStateChange,
			TaskID:    strings.TrimSpace(envGetString(env, "task.id")),
			Message:   "insertion decision recorded",
			Timestamp: time.Now().UTC(),
			Metadata:  capruntime.RedactMetadataMap(metadata),
		})
	}
	if res != nil {
		if res.Metadata == nil {
			res.Metadata = map[string]any{}
		}
		res.Metadata["insertion_decision"] = envelope.Insertion
	}
	return envelope
}

func (n *reactActNode) recordObservation(ctx context.Context, env *contextdata.Envelope, call model.ToolCall, res *ports.ToolResult, envelope *capresult.CapabilityResultEnvelope) {
	if n == nil {
		return
	}
	agent := n.agent
	if agent == nil {
		return
	}
	task := n.task
	if task == nil {
		return
	}
	appendToolMessage(agent, task, env, call, res, envelope)
	decision := resolveInsertionDecision(agent, task, envelope)
	observation := summarizeToolResult(env, call, res, decision)
	displaySummary, visible := renderInsertionFilteredSummary(agent, task, call.Name, res, envelope)
	if visible {
		observation.Summary = displaySummary
		switch decision.Action {
		case agentspec.InsertionActionMetadataOnly, agentspec.InsertionActionHITLRequired:
			observation.Data = nil
		}
	}
	history := getToolObservations(env)
	if visible {
		history = append(history, observation)
		limit := toolSummaryBudgetForPhase(envGetString(env, "react.phase"))
		if len(history) > limit {
			history = history[len(history)-limit:]
		}
	}
	env.SetWorkingValueWithClass("react.tool_observations", history, contextdata.MemoryClassTask)
	if n != nil && n.agent != nil && n.agent.outputIngestionEnabled() {
		summary := strings.TrimSpace(observation.Summary)
		knowledge.IngestObservationAsync(contextdata.WithEnvelope(ctx, env), n.agent.OutputIngester, summary)
	}
	// TODO: ContextManager integration requires framework-level fixes for missing types
	// (core.ToolResultContextItem, core.FileContextItem)
	// if visible && n.agent.contextPolicy != nil && n.agent.contextPolicy.ContextManager != nil {
	// 	summaryEnvelope := capresult.SummarizeCapabilityResultEnvelope(envelope, observation.Summary)
	// 	item := &core.ToolResultContextItem{
	// 		ToolName:     call.Name,
	// 		Result:       &ports.ToolResult{Success: res.Success, Data: map[string]interface{}{"summary": observation.Summary}, Error: res.Error},
	// 		Envelope:     summaryEnvelope,
	// 		LastAccessed: time.Now().UTC(),
	// 		Relevance:    0.9,
	// 		PriorityVal:  1,
	// 	}
	// 	_ = n.agent.contextPolicy.ContextManager.AddItem(item)
	// 	if call.Name == "file_read" {
	// 		path := fmt.Sprint(call.Args["path"])
	// 		snippet := observation.Data["snippet"]
	// 		if path != "" && fmt.Sprint(snippet) != "" {
	// 			_ = n.agent.contextPolicy.ContextManager.UpsertFileItem(&core.FileContextItem{
	// 				Path:         path,
	// 				Content:      fmt.Sprint(snippet),
	// 				Summary:      fmt.Sprint(snippet),
	// 				LastAccessed: time.Now().UTC(),
	// 				Relevance:    1.0,
	// 				PriorityVal:  0,
	// 			})
	// 		}
	// 	}
	// }
}

func (n *reactActNode) refreshIndexesAfterMutation(ctx context.Context, call model.ToolCall, res *ports.ToolResult) {
	if n == nil || n.agent == nil || res == nil || !res.Success {
		return
	}
	paths := mutationPaths(call, res)
	if len(paths) == 0 {
		return
	}
	if n.agent.IndexManager != nil {
		if err := n.agent.IndexManager.RefreshFiles(ctx, paths); err != nil {
			n.agent.debugf("ast index refresh failed for %v: %v", paths, err)
		}
	}
	if n.agent.SearchEngine != nil {
		if err := n.agent.SearchEngine.RefreshFiles(paths); err != nil {
			n.agent.debugf("search index refresh failed for %v: %v", paths, err)
		}
	}
}

func mutationPaths(call model.ToolCall, res *ports.ToolResult) []string {
	name := strings.TrimSpace(call.Name)
	switch name {
	case "file_write", "file_create":
		if path := resultPathOrArg(call, res); path != "" {
			return []string{path}
		}
	case "file_delete":
		if path := fmt.Sprint(call.Args["path"]); strings.TrimSpace(path) != "" {
			return []string{path}
		}
	}
	return nil
}

func resultPathOrArg(call model.ToolCall, res *ports.ToolResult) string {
	if res != nil && res.Data != nil {
		if path := strings.TrimSpace(fmt.Sprint(res.Data["path"])); path != "" && path != "<nil>" {
			return path
		}
	}
	path := strings.TrimSpace(fmt.Sprint(call.Args["path"]))
	if path == "<nil>" {
		return ""
	}
	return path
}
