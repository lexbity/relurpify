package agenttest

import (
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/named/rex"
	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// CapabilityCoverage tracks which registered capabilities were exercised
type CapabilityCoverage struct {
	RegistryID       string         `json:"registry_id"`
	RegisteredTools  []string       `json:"registered_tools"`
	ExercisedTools   map[string]int `json:"exercised_tools"`
	UnexercisedTools []string       `json:"unexercised_tools"`
	CoverageRatio    float64        `json:"coverage_ratio"`
}

// ExtractCapabilityRegistry introspects the agent to discover available tools
func ExtractCapabilityRegistry(agent graph.WorkflowExecutor) (*CapabilityCoverage, error) {
	if agent == nil {
		return nil, fmt.Errorf("agent is nil")
	}

	reg := extractCapabilityRegistry(agent)
	if reg == nil {
		return nil, fmt.Errorf("agent has no capability registry")
	}

	coverage := &CapabilityCoverage{
		RegistryID:      fmt.Sprintf("%p", reg),
		ExercisedTools:  make(map[string]int),
		RegisteredTools: make([]string, 0),
	}

	// Collect all registered tools (use Name to match telemetry format)
	for _, desc := range reg.CallableCapabilities() {
		name := desc.Name
		if name == "" {
			// Fallback to ID, stripping "tool:" prefix if present
			name = strings.TrimPrefix(desc.ID, "tool:")
		}
		if name != "" {
			coverage.RegisteredTools = append(coverage.RegisteredTools, name)
		}
	}

	sort.Strings(coverage.RegisteredTools)
	return coverage, nil
}

// ComputeCoverage compares exercised tools against registry and updates the coverage struct
func ComputeCoverage(coverage *CapabilityCoverage, toolCounts map[string]int) error {
	if coverage == nil {
		return fmt.Errorf("coverage is nil")
	}

	if coverage.ExercisedTools == nil {
		coverage.ExercisedTools = make(map[string]int)
	}

	// Copy exercised tool counts
	for tool, count := range toolCounts {
		if count > 0 {
			coverage.ExercisedTools[tool] = count
		}
	}

	// Calculate unexercised tools
	coverage.UnexercisedTools = nil
	registeredSet := make(map[string]struct{}, len(coverage.RegisteredTools))
	for _, tool := range coverage.RegisteredTools {
		registeredSet[tool] = struct{}{}
	}

	for tool := range registeredSet {
		if _, exercised := coverage.ExercisedTools[tool]; !exercised {
			coverage.UnexercisedTools = append(coverage.UnexercisedTools, tool)
		}
	}
	sort.Strings(coverage.UnexercisedTools)

	// Calculate coverage ratio
	if len(coverage.RegisteredTools) > 0 {
		coverage.CoverageRatio = float64(len(coverage.ExercisedTools)) / float64(len(coverage.RegisteredTools))
	} else {
		coverage.CoverageRatio = 0.0
	}

	return nil
}

// RegistryHasTool checks if a tool is registered in the capability registry
func RegistryHasTool(agent graph.WorkflowExecutor, toolName string) bool {
	if agent == nil || toolName == "" {
		return false
	}

	reg := extractCapabilityRegistry(agent)
	if reg == nil {
		return false
	}

	_, found := reg.Get(toolName)
	return found
}

// extractCapabilityRegistry gets the capability registry from an agent
func extractCapabilityRegistry(agent graph.WorkflowExecutor) *capability.Registry {
	type capabilityRegistryProvider interface {
		CapabilityRegistry() *capability.Registry
	}
	if provider, ok := agent.(capabilityRegistryProvider); ok {
		return provider.CapabilityRegistry()
	}
	if rexAgent, ok := agent.(*rex.Agent); ok && rexAgent.Environment != nil {
		return rexAgent.Environment.Registry
	}
	return nil
}

// CoverageReport generates a human-readable coverage report
func CoverageReport(coverage *CapabilityCoverage) string {
	if coverage == nil {
		return "Coverage: nil"
	}

	report := "Capability Coverage Report\n"
	report += "===========================\n"
	report += fmt.Sprintf("Registry ID: %s\n", coverage.RegistryID)
	report += fmt.Sprintf("Total Tools: %d\n", len(coverage.RegisteredTools))
	report += fmt.Sprintf("Exercised Tools: %d\n", len(coverage.ExercisedTools))
	report += fmt.Sprintf("Unexercised Tools: %d\n", len(coverage.UnexercisedTools))
	report += fmt.Sprintf("Coverage Ratio: %.1f%%\n", coverage.CoverageRatio*100)

	if len(coverage.UnexercisedTools) > 0 {
		report += "\nUnexercised Tools:\n"
		for _, tool := range coverage.UnexercisedTools {
			report += fmt.Sprintf("  - %s\n", tool)
		}
	}

	if len(coverage.ExercisedTools) > 0 {
		report += "\nExercised Tools:\n"
		// Sort exercised tools for consistent output
		var exercised []string
		for tool := range coverage.ExercisedTools {
			exercised = append(exercised, tool)
		}
		sort.Strings(exercised)
		for _, tool := range exercised {
			report += fmt.Sprintf("  - %s (called %d times)\n", tool, coverage.ExercisedTools[tool])
		}
	}

	return report
}

// ValidateToolsRequired checks if all required tools were exercised during the test
func ValidateToolsRequired(coverage *CapabilityCoverage, requiredTools []string) []string {
	var failures []string
	if coverage == nil {
		if len(requiredTools) > 0 {
			return []string{"cannot validate required tools: coverage is nil"}
		}
		return nil
	}

	for _, tool := range requiredTools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			continue
		}
		if count, found := coverage.ExercisedTools[tool]; !found || count == 0 {
			failures = append(failures, fmt.Sprintf("required tool %s was not exercised", tool))
		}
	}

	return failures
}

// BuildCoverageFromEvents creates a CapabilityCoverage from telemetry events
func BuildCoverageFromEvents(agent graph.WorkflowExecutor, events []core.Event) (*CapabilityCoverage, error) {
	coverage, err := ExtractCapabilityRegistry(agent)
	if err != nil {
		return nil, err
	}

	// Extract tool counts from events
	_, toolCounts := CountToolCalls(events)

	// Compute coverage
	if err := ComputeCoverage(coverage, toolCounts); err != nil {
		return nil, err
	}

	return coverage, nil
}
