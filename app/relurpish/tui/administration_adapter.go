package tui

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// InspectConfiguration captures one runtime, so a workspace swap cannot mix
// two sets of policy/catalog observations into the same inspection document.
func (r *runtimeAdapter) InspectConfiguration(ctx context.Context) (InspectionDocument, error) {
	if err := ctx.Err(); err != nil {
		return InspectionDocument{}, err
	}
	live := r.currentRuntime()
	if live == nil {
		return InspectionDocument{}, fmt.Errorf("runtime unavailable")
	}
	reader := &runtimeAdapter{}
	reader.rt.Store(live)
	document := InspectionDocument{Title: "Effective configuration", Notice: "Observed runtime declarations/catalogs. Invocation-time authorization and actual prompt use are separate checks."}
	policies := reader.GetClassPolicies()
	classes := make([]string, 0, len(policies))
	for class := range policies {
		classes = append(classes, class)
	}
	sort.Strings(classes)
	for _, class := range classes {
		document.Entries = append(document.Entries, InspectionEntry{ID: "policy:" + class, Label: "Policy / " + class, Origin: live.Config.Workspace, Fields: []InspectionField{{Name: "Declared level", Value: string(policies[class])}}})
	}
	for _, capability := range reader.ListCapabilities() {
		document.Entries = append(document.Entries, InspectionEntry{ID: "capability:" + capability.ID, Label: "Capability / " + capability.Name, Origin: capability.ProviderID, Summary: capability.Description, Fields: []InspectionField{{Name: "ID", Value: capability.ID}, {Name: "Kind", Value: capability.Kind}, {Name: "Runtime", Value: capability.RuntimeFamily}, {Name: "Exposure", Value: capability.Exposure}, {Name: "Scope", Value: capability.Scope}}})
	}
	for _, prompt := range reader.ListPrompts() {
		entry := InspectionEntry{ID: "prompt:" + prompt.PromptID, Label: "Prompt / " + prompt.PromptID, Origin: prompt.Meta.Source, Summary: "Registered reference/preview; actual-call use is not captured here.", Fields: []InspectionField{{Name: "Provider", Value: prompt.ProviderID}, {Name: "Variables", Value: strings.Join(prompt.Variables, ", ")}}}
		detail, err := reader.GetPromptDetail(prompt.PromptID)
		if err != nil {
			entry.Body = "Preview unavailable: " + err.Error()
		} else if detail != nil {
			var lines []string
			for _, message := range detail.Messages {
				lines = append(lines, message.Role)
				for _, block := range message.Content {
					lines = append(lines, renderStructuredContentPreview(block)...)
				}
			}
			entry.Body = strings.Join(lines, "\n")
		}
		document.Entries = append(document.Entries, entry)
	}
	for _, tool := range reader.ListToolsInfo() {
		document.Entries = append(document.Entries, InspectionEntry{ID: "tool:" + tool.Name, Label: "Tool / " + tool.Name, Origin: live.Config.Workspace, Fields: []InspectionField{{Name: "Runtime", Value: tool.RuntimeFamily}, {Name: "Scope", Value: tool.Scope}, {Name: "Policy override", Value: string(tool.Policy)}, {Name: "Exposure", Value: tool.Exposure}, {Name: "Risk classes", Value: strings.Join(tool.RiskClasses, ", ")}}})
	}
	if contract := reader.ContractSummary(); contract != nil {
		document.Entries = append(document.Entries, InspectionEntry{ID: "contract", Label: "Resolved contract", Origin: contract.Workspace, Fields: []InspectionField{{Name: "Agent", Value: contract.AgentID}, {Name: "Manifest", Value: contract.ManifestName + " " + contract.ManifestVersion}, {Name: "Capabilities", Value: fmt.Sprint(contract.CapabilityCount)}, {Name: "Applied skills", Value: strings.Join(contract.AppliedSkills, ", ")}, {Name: "Failed skills", Value: strings.Join(contract.FailedSkills, ", ")}}})
	}
	return document, nil
}

func (r *runtimeAdapter) Services(ctx context.Context) ([]ServiceInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	live := r.currentRuntime()
	if live == nil || live.AgentWorkspace() == nil || live.AgentWorkspace().ServiceManager == nil {
		return nil, fmt.Errorf("service manager unavailable")
	}
	var result []ServiceInfo
	for _, snapshot := range live.AgentWorkspace().ServiceManager.Snapshot() {
		result = append(result, ServiceInfo{ID: snapshot.ID, Status: ServiceStatus(snapshot.Status), Source: snapshot.Source, Owner: snapshot.Owner, Notes: slices.Clone(snapshot.Notes)})
	}
	return result, nil
}

// ControlServices acts only on reviewed IDs in the reviewed workspace. It
// captures the manager before effects and retains every individual failure.
func (r *runtimeAdapter) ControlServices(ctx context.Context, operation ServiceOperation) (ServiceOutcome, error) {
	live := r.currentRuntime()
	if live == nil || live.AgentWorkspace() == nil || live.AgentWorkspace().ServiceManager == nil {
		return ServiceOutcome{}, fmt.Errorf("service manager unavailable")
	}
	if operation.Workspace != live.Config.Workspace {
		return ServiceOutcome{}, fmt.Errorf("workspace changed; refresh and review the service operation again")
	}
	if operation.Action != "stop" && operation.Action != "restart" && operation.Action != "restart-all" {
		return ServiceOutcome{}, fmt.Errorf("unknown service action")
	}
	manager := live.AgentWorkspace().ServiceManager
	registered := make(map[string]bool)
	for _, item := range manager.Snapshot() {
		registered[item.ID] = true
	}
	seen := make(map[string]bool)
	if len(operation.IDs) == 0 {
		return ServiceOutcome{}, fmt.Errorf("reviewed service IDs required")
	}
	for _, id := range operation.IDs {
		if seen[id] || !registered[id] {
			return ServiceOutcome{}, fmt.Errorf("service set changed or contains duplicates: %s", id)
		}
		seen[id] = true
	}
	var outcome ServiceOutcome
	for _, id := range operation.IDs {
		err := ctx.Err()
		if err == nil {
			if operation.Action == "stop" {
				err = manager.Stop(id)
			} else {
				err = manager.Restart(ctx, id)
			}
		}
		result := ServiceOperationResult{ID: id}
		if err != nil {
			result.Error = err.Error()
		}
		outcome.Results = append(outcome.Results, result)
	}
	for _, snapshot := range manager.Snapshot() {
		outcome.Services = append(outcome.Services, ServiceInfo{ID: snapshot.ID, Status: ServiceStatus(snapshot.Status), Source: snapshot.Source, Owner: snapshot.Owner, Notes: slices.Clone(snapshot.Notes)})
	}
	return outcome, nil
}
