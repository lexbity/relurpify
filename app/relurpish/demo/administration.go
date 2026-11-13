package demo

import (
	"context"
	"fmt"
	"slices"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
)

// DEMO ONLY: Supplies scenario policy/tool/prompt metadata, plus real installed
// cognition-zoo contract names. It does not describe a live execution runtime.
// LIVE CONTRACT: ConfigurationReader.InspectConfiguration.
// REPLACEMENT: Supply the selected live runtime's immutable observation document.
// SIDE EFFECTS: Demo state only; no files, services, model calls, or grants.
func (s *Session) InspectConfiguration(ctx context.Context) (tui.InspectionDocument, error) {
	catalog, err := s.RecipeCatalog(ctx)
	if err != nil {
		return tui.InspectionDocument{}, err
	}
	document := tui.InspectionDocument{Title: "Effective configuration", Notice: "DEMO: illustrative configuration. Registered references/previews do not establish actual prompt use or invocation authorization."}
	document.Entries = []tui.InspectionEntry{
		{ID: "policy:workspace", Label: "Policy / Workspace writes", Origin: "DEMO memory", Fields: []tui.InspectionField{{Name: "Declared level", Value: "ask"}, {Name: "Scope", Value: "workspace files"}}},
		{ID: "tool:file_read", Label: "Tool / file_read", Origin: "DEMO catalog", Summary: "A visible tool is not a permission grant", Fields: []tui.InspectionField{{Name: "Exposure", Value: "model-callable"}}},
	}
	for _, prompt := range catalog.Prompts {
		document.Entries = append(document.Entries, tui.InspectionEntry{ID: "prompt:" + prompt.ID, Label: "Prompt / " + prompt.ID, Origin: prompt.Origin, Summary: "Available reference and preview; actual use is not established", Body: prompt.Text, Fields: []tui.InspectionField{{Name: "Version", Value: prompt.Version}}})
	}
	for _, strategy := range catalog.Strategies {
		document.Entries = append(document.Entries, tui.InspectionEntry{ID: "strategy:" + strategy.Name, Label: "Strategy / " + strategy.Name, Origin: "Installed cognition-zoo contract registry", Fields: []tui.InspectionField{{Name: "Execution shape", Value: strategy.Shape}}})
	}
	return document, nil
}

// DEMO ONLY: Observes in-memory registered-service states.
// LIVE CONTRACT: ServicePort.Services.
// REPLACEMENT: Supply live service-manager observations; keep lifecycle UI coverage.
// SIDE EFFECTS: Demo state only; no files, services, model calls, or grants.
func (s *Session) Services(ctx context.Context) ([]tui.ServiceInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneServices(s.services), nil
}

// DEMO ONLY: Changes only the reviewed in-memory service IDs. The scenario
// services-partial fails knowledge.index and retains the other successful states.
// LIVE CONTRACT: ServicePort.ControlServices.
// REPLACEMENT: Supply real scoped manager results; retain partial-failure/review tests.
// SIDE EFFECTS: Demo state only; no files, services, model calls, or grants.
func (s *Session) ControlServices(ctx context.Context, operation tui.ServiceOperation) (tui.ServiceOutcome, error) {
	if err := ctx.Err(); err != nil {
		return tui.ServiceOutcome{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if operation.Action != "stop" && operation.Action != "restart" && operation.Action != "restart-all" {
		return tui.ServiceOutcome{}, fmt.Errorf("unknown service operation")
	}
	if len(operation.IDs) == 0 {
		return tui.ServiceOutcome{}, fmt.Errorf("reviewed service IDs are required")
	}
	seen := make(map[string]bool)
	for _, id := range operation.IDs {
		if seen[id] || !slices.ContainsFunc(s.services, func(item tui.ServiceInfo) bool { return item.ID == id }) {
			return tui.ServiceOutcome{}, fmt.Errorf("service scope changed: %s", id)
		}
		seen[id] = true
	}
	var outcome tui.ServiceOutcome
	for _, id := range operation.IDs {
		index := slices.IndexFunc(s.services, func(item tui.ServiceInfo) bool { return item.ID == id })
		result := tui.ServiceOperationResult{ID: id}
		if operation.Action == "stop" {
			s.services[index].Status = tui.ServiceStatusStopped
		} else if s.scenario == "services-partial" && id == "knowledge.index" {
			s.services[index].Status = tui.ServiceStatusError
			result.Error = "DEMO: index initialization failed"
			s.services[index].Notes = []string{result.Error}
		} else {
			s.services[index].Status = tui.ServiceStatusRunning
			s.services[index].Notes = nil
		}
		outcome.Results = append(outcome.Results, result)
	}
	outcome.Services = cloneServices(s.services)
	return outcome, nil
}

func cloneServices(items []tui.ServiceInfo) []tui.ServiceInfo {
	copy := slices.Clone(items)
	for i := range copy {
		copy[i].Notes = slices.Clone(copy[i].Notes)
	}
	return copy
}
