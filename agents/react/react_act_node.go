package react

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type reactActNode struct {
	id    string
	agent *ReActAgent
	task  *core.Task
}

// ID returns the node identifier for the "act" step.
func (n *reactActNode) ID() string { return n.id }

// Type labels the node as a tool execution step.
func (n *reactActNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeTool }

// Contract marks the ReAct act step as a capability-consuming execution node.
func (n *reactActNode) Contract() agentgraph.NodeContract {
	return agentgraph.NodeContract{
		RequiredCapabilities: []core.CapabilitySelector{{
			Kind: core.CapabilityKindTool,
		}},
		SideEffectClass: agentgraph.SideEffectExternal,
		Idempotency:     agentgraph.IdempotencyUnknown,
		ContextPolicy: core.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "react.decision", "react.tool_calls", "react.*"},
			WriteKeys:                []string{"react.last_tool_result", "react.last_tool_result_*", "react.tool_observations", "react.*"},
			AllowHistoryAccess:       true,
			AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassWorking},
			AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassStepMetadata, core.StateDataClassArtifactRef, core.StateDataClassMemoryRef, core.StateDataClassStructuredState},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 16,
			PreferArtifactReferences: true,
		},
	}
}

// Execute runs any pending tool calls or directly invokes the requested tool
// referenced in the latest decision payload.
func (n *reactActNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	env.SetWorkingValue("react.execution_phase", "executing", contextdata.MemoryClassTask)
	activeTools := activeToolSet(env)
	if pending, ok := env.GetWorkingValue("react.tool_calls"); ok {
		if calls, ok := pending.([]contracts.ToolCall); ok && len(calls) > 0 {
			calls = filterToolCalls(calls)
			if len(calls) == 0 {
				env.SetWorkingValue("react.tool_calls", []contracts.ToolCall{}, contextdata.MemoryClassTask)
			} else {
				results := make(map[string]interface{})
				envelopes := make(map[string]*core.CapabilityResultEnvelope)
				toolErrors := make([]string, 0)
				overallSuccess := true
				for _, call := range calls {
					if !n.capabilityAllowed(call.Name, activeTools) || !n.agent.Tools.HasCapability(call.Name) {
						errResult := &contracts.ToolResult{
							Success: false,
							Error:   fmt.Sprintf("tool %q does not exist. Only use tools from the available list.", call.Name),
						}
						envelope := n.capabilityEnvelope(ctx, env, nil, call, errResult)
						n.recordObservation(env, call, errResult, envelope)
						envelopes[call.Name] = envelope
						overallSuccess = false
						toolErrors = append(toolErrors, fmt.Sprintf("unknown tool %s", call.Name))
						continue
					}
					if !n.agent.Tools.CapabilityAvailable(ctx, env, call.Name) {
						errResult := &contracts.ToolResult{
							Success: false,
							Error:   fmt.Sprintf("tool %q is unavailable right now.", call.Name),
						}
						envelope := n.capabilityEnvelope(ctx, env, nil, call, errResult)
						n.recordObservation(env, call, errResult, envelope)
						envelopes[call.Name] = envelope
						overallSuccess = false
						toolErrors = append(toolErrors, fmt.Sprintf("unavailable tool %s", call.Name))
						continue
					}
					n.agent.debugf("%s executing tool=%s args=%v", n.id, call.Name, call.Args)
					res, err := n.agent.Tools.InvokeCapability(ctx, env, call.Name, call.Args)
					if err != nil {
						// Convert hard tool errors (e.g. schema validation, permission denial)
						// into soft ToolResult failures so the LLM can observe and recover.
						res = &contracts.ToolResult{Success: false, Error: err.Error()}
						err = nil
					}
					if res != nil {
						envelope := n.capabilityEnvelope(ctx, env, nil, call, res)
						envelopes[call.Name] = envelope
						n.recordObservation(env, call, res, envelope)
						n.latchVerificationSuccess(env, call.Name, res)
						n.refreshIndexesAfterMutation(call, res)
						results[call.Name] = map[string]interface{}{
							"success": res.Success,
							"data":    res.Data,
							"error":   res.Error,
						}
						n.agent.debugf("%s tool=%s result=%v", n.id, call.Name, res.Data)
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
				env.SetWorkingValue("react.last_tool_result", results, contextdata.MemoryClassTask)
				env.SetWorkingValue("react.last_tool_result_envelopes", envelopes, contextdata.MemoryClassTask)
				env.SetWorkingValue("react.tool_calls", []contracts.ToolCall{}, contextdata.MemoryClassTask)
				result := &core.Result{
					NodeID:  n.id,
					Success: overallSuccess,
					Data:    results,
					Metadata: map[string]any{
						"capability_results": envelopes,
					},
				}
				if len(toolErrors) > 0 {
					result.Error = strings.Join(toolErrors, "; ")
				}
				env.SetWorkingValue("react.last_result", result, contextdata.MemoryClassTask)
				return result, nil
			}
		}
		if n.agent.Config != nil && !n.agent.Config.NativeToolCalling {
			env.SetWorkingValue("react.tool_calls", []contracts.ToolCall{}, contextdata.MemoryClassTask)
		}
	}
	val, ok := env.GetWorkingValue("react.decision")
	if !ok {
		return nil, fmt.Errorf("missing decision from think step")
	}
	decision := val.(decisionPayload)
	toolName := strings.TrimSpace(decision.Tool)
	if decision.Complete || toolName == "" || strings.EqualFold(toolName, "none") {
		env.SetWorkingValue("react.last_tool_result", map[string]interface{}{}, contextdata.MemoryClassTask)
		result := &core.Result{NodeID: n.id, Success: true}
		env.SetWorkingValue("react.last_result", result, contextdata.MemoryClassTask)
		return result, nil
	}
	if !n.capabilityAllowed(toolName, activeTools) || !n.agent.Tools.HasCapability(toolName) {
		lower := strings.ToLower(toolName)
		if lower == "" || strings.Contains(lower, "none") {
			env.SetWorkingValue("react.last_tool_result", map[string]interface{}{}, contextdata.MemoryClassTask)
			result := &core.Result{NodeID: n.id, Success: true}
			env.SetWorkingValue("react.last_result", result, contextdata.MemoryClassTask)
			return result, nil
		}
		// Feed error back to the LLM so it can retry with a valid tool name.
		errMsg := fmt.Sprintf("tool %q does not exist. Only use tools from the available list.", toolName)
		env.SetWorkingValue("react.last_tool_result", map[string]interface{}{"error": errMsg}, contextdata.MemoryClassTask)
		result := &core.Result{NodeID: n.id, Success: false, Error: errMsg}
		env.SetWorkingValue("react.last_result", result, contextdata.MemoryClassTask)
		return result, nil
	}
	if !n.agent.Tools.CapabilityAvailable(ctx, env, toolName) {
		errMsg := fmt.Sprintf("tool %q is unavailable right now.", toolName)
		env.SetWorkingValue("react.last_tool_result", map[string]interface{}{"error": errMsg}, contextdata.MemoryClassTask)
		result := &core.Result{NodeID: n.id, Success: false, Error: errMsg}
		env.SetWorkingValue("react.last_result", result, contextdata.MemoryClassTask)
		return result, nil
	}
	res, err := n.agent.Tools.InvokeCapability(ctx, env, toolName, decision.Arguments)
	if err != nil {
		return nil, err
	}
	call := contracts.ToolCall{
		ID:   NewUUID(),
		Name: decision.Tool,
		Args: decision.Arguments,
	}
	envelope := n.capabilityEnvelope(ctx, env, nil, call, res)
	n.recordObservation(env, call, res, envelope)
	n.latchVerificationSuccess(env, call.Name, res)
	env.SetWorkingValue("react.last_tool_result", res.Data, contextdata.MemoryClassTask)
	env.SetWorkingValue("react.last_tool_result_envelope", envelope, contextdata.MemoryClassTask)
	n.agent.debugf("%s tool=%s result=%v", n.id, decision.Tool, res.Data)
	result := &core.Result{
		NodeID:  n.id,
		Success: res.Success,
		Data:    res.Data,
		Metadata: map[string]any{
			"capability_result": envelope,
		},
		Error: strings.TrimSpace(res.Error),
	}
	n.refreshIndexesAfterMutation(call, res)
	env.SetWorkingValue("react.last_result", result, contextdata.MemoryClassTask)
	return result, nil
}

func (n *reactActNode) latchVerificationSuccess(env *contextdata.Envelope, toolName string, res *contracts.ToolResult) {
	if env == nil || n == nil || n.agent == nil || n.task == nil || res == nil || !res.Success {
		return
	}
	if !taskNeedsEditing(n.task) || !verificationStopAllowed(n.agent, n.task) {
		return
	}
	// Allow the latch even when no prior file edit was observed — the agent
	// may be verifying already-correct code (verify-only pass, no edits needed).
	if !verificationToolMatches(toolName, n.agent.verificationSuccessTools(n.task)) {
		return
	}
	summary := verificationSuccessSummary(toolName, fmt.Sprint(res.Data["stdout"]))
	env.SetWorkingValue("react.verification_latched_summary", summary, contextdata.MemoryClassTask)
	env.SetWorkingValue("react.synthetic_summary", summary, contextdata.MemoryClassTask)
	env.SetWorkingValue("react.incomplete_reason", "", contextdata.MemoryClassTask)
}

func (n *reactActNode) capabilityAllowed(name string, active map[string]struct{}) bool {
	if len(active) > 0 {
		if _, ok := active[name]; !ok {
			return false
		}
	}
	return true
}

func (n *reactActNode) capabilityEnvelope(ctx context.Context, env *contextdata.Envelope, tool contracts.Tool, call contracts.ToolCall, res *contracts.ToolResult) *core.CapabilityResultEnvelope {
	var desc core.CapabilityDescriptor
	if res != nil && res.Metadata != nil {
		if raw, ok := res.Metadata["capability_descriptor"]; ok {
			if typed, ok := raw.(core.CapabilityDescriptor); ok {
				desc = typed
			}
		}
	}
	if desc.ID == "" {
		if n != nil && n.agent != nil {
			if resolved, ok := n.agent.executionCapabilityDescriptor(call.Name); ok {
				desc = resolved
			}
		}
	}
	if desc.ID == "" {
		if tool != nil {
			desc = core.ToolDescriptor(ctx, tool)
		} else {
			desc = core.CapabilityDescriptor{
				ID:          "tool:" + call.Name,
				Kind:        core.CapabilityKindTool,
				Name:        call.Name,
				Description: call.Name,
				TrustClass:  core.TrustClassWorkspaceTrusted,
				Source: core.CapabilitySource{
					Scope: core.CapabilityScopeWorkspace,
				},
			}
		}
	}
	var approval *core.ApprovalBinding
	if res != nil && res.Metadata != nil {
		if raw, ok := res.Metadata["approval_binding"]; ok {
			if typed, ok := raw.(*core.ApprovalBinding); ok {
				approval = typed
			}
		}
	}
	if approval == nil {
		// ApprovalBindingFromCapability already works with envelope WorkingData
		approval = core.ApprovalBindingFromCapability(desc, env.WorkingData, call.Args)
	}
	var snapshot *core.PolicySnapshot
	if n != nil && n.agent != nil {
		snapshot = n.agent.executionPolicySnapshot()
	}
	envelope := core.NewCapabilityResultEnvelope(desc, res, core.ContentDispositionRaw, snapshot, approval)
	envelope = core.ApplyInsertionDecision(envelope, resolveInsertionDecision(n.agent, n.task, envelope))
	if n != nil && n.agent != nil && n.agent.Config != nil && n.agent.Config.Telemetry != nil {
		metadata := map[string]interface{}{
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
		n.agent.Config.Telemetry.Emit(core.Event{
			Type:      core.EventStateChange,
			TaskID:    strings.TrimSpace(envGetString(env, "task.id")),
			Message:   "insertion decision recorded",
			Timestamp: time.Now().UTC(),
			Metadata:  core.RedactMetadataMap(metadata),
		})
	}
	if res != nil {
		if res.Metadata == nil {
			res.Metadata = map[string]interface{}{}
		}
		res.Metadata["insertion_decision"] = envelope.Insertion
	}
	return envelope
}

func (n *reactActNode) recordObservation(env *contextdata.Envelope, call contracts.ToolCall, res *contracts.ToolResult, envelope *core.CapabilityResultEnvelope) {
	appendToolMessage(n.agent, n.task, env, call, res, envelope)
	observation := summarizeToolResult(env, call, res)
	displaySummary, visible := renderInsertionFilteredSummary(n.agent, n.task, call.Name, res, envelope)
	if visible {
		observation.Summary = displaySummary
		switch resolveInsertionDecision(n.agent, n.task, envelope).Action {
		case core.InsertionActionMetadataOnly, core.InsertionActionHITLRequired:
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
	env.SetWorkingValue("react.tool_observations", history, contextdata.MemoryClassTask)
	if n != nil && n.agent != nil && n.agent.outputIngestionEnabled() {
		summary := strings.TrimSpace(observation.Summary)
		knowledge.IngestObservationAsync(contextdata.WithEnvelope(context.Background(), env), n.agent.OutputIngester, summary)
	}
	// TODO: ContextManager integration requires framework-level fixes for missing types
	// (core.ToolResultContextItem, core.FileContextItem)
	// if visible && n.agent.contextPolicy != nil && n.agent.contextPolicy.ContextManager != nil {
	// 	summaryEnvelope := core.SummarizeCapabilityResultEnvelope(envelope, observation.Summary)
	// 	item := &core.ToolResultContextItem{
	// 		ToolName:     call.Name,
	// 		Result:       &contracts.ToolResult{Success: res.Success, Data: map[string]interface{}{"summary": observation.Summary}, Error: res.Error},
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

func (n *reactActNode) refreshIndexesAfterMutation(call contracts.ToolCall, res *contracts.ToolResult) {
	if n == nil || n.agent == nil || res == nil || !res.Success {
		return
	}
	paths := mutationPaths(call, res)
	if len(paths) == 0 {
		return
	}
	if n.agent.IndexManager != nil {
		if err := n.agent.IndexManager.RefreshFiles(paths); err != nil {
			n.agent.debugf("ast index refresh failed for %v: %v", paths, err)
		}
	}
	if n.agent.SearchEngine != nil {
		if err := n.agent.SearchEngine.RefreshFiles(paths); err != nil {
			n.agent.debugf("search index refresh failed for %v: %v", paths, err)
		}
	}
}

func mutationPaths(call contracts.ToolCall, res *contracts.ToolResult) []string {
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

func resultPathOrArg(call contracts.ToolCall, res *contracts.ToolResult) string {
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
