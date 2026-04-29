package agenttest

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// ToolResponseOverride configures synthetic tool responses for testing failure modes
type ToolResponseOverride struct {
	Tool        string                 `yaml:"tool"`
	MatchArgs   map[string]interface{} `yaml:"match_args,omitempty"`
	Response    *core.ToolResult       `yaml:"response,omitempty"`
	Error       string                 `yaml:"error,omitempty"`
	FailureRate float64                `yaml:"failure_rate,omitempty"`
	LatencyMs   int                    `yaml:"latency_ms,omitempty"`
	CallCount   int                    `yaml:"call_count,omitempty"`
}

// InjectionInterceptor wraps tool execution with override logic
type InjectionInterceptor struct {
	base        core.Tool
	overrides   []ToolResponseOverride
	callHistory map[string]int
	mu          sync.Mutex
}

// NewInjectionInterceptor creates an interceptor for a tool with override rules
func NewInjectionInterceptor(base core.Tool, overrides []ToolResponseOverride) *InjectionInterceptor {
	return &InjectionInterceptor{
		base:        base,
		overrides:   filterOverridesForTool(overrides, base.Name()),
		callHistory: make(map[string]int),
	}
}

// Name returns the wrapped tool's name
func (i *InjectionInterceptor) Name() string {
	return i.base.Name()
}

// Description returns the wrapped tool's description
func (i *InjectionInterceptor) Description() string {
	return i.base.Description()
}

// Category returns the wrapped tool's category
func (i *InjectionInterceptor) Category() string {
	return i.base.Category()
}

// Parameters returns the wrapped tool's parameters
func (i *InjectionInterceptor) Parameters() []core.ToolParameter {
	return i.base.Parameters()
}

// Tags returns the wrapped tool's tags
func (i *InjectionInterceptor) Tags() []string {
	return i.base.Tags()
}

// Permissions returns the wrapped tool's permissions
func (i *InjectionInterceptor) Permissions() core.ToolPermissions {
	return i.base.Permissions()
}

// IsAvailable delegates to the wrapped tool
func (i *InjectionInterceptor) IsAvailable(ctx context.Context) bool {
	return i.base.IsAvailable(ctx)
}

// Execute wraps the tool execution with injection logic
func (i *InjectionInterceptor) Execute(ctx context.Context, args map[string]interface{}) (*core.ToolResult, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	toolName := i.base.Name()
	i.callHistory[toolName]++
	callNum := i.callHistory[toolName]

	// Find matching override
	for _, override := range i.overrides {
		if !i.matchesOverride(override, args, callNum) {
			continue
		}

		// Apply latency injection
		if override.LatencyMs > 0 {
			time.Sleep(time.Duration(override.LatencyMs) * time.Millisecond)
		}

		// Apply failure rate injection
		if override.FailureRate > 0 && rand.Float64() < override.FailureRate {
			return i.createErrorResult(override, "injected failure"), nil
		}

		// Apply explicit error
		if override.Error != "" {
			return i.createErrorResult(override, override.Error), nil
		}

		// Apply explicit response
		if override.Response != nil {
			return override.Response, nil
		}
	}

	// No override matched, execute base tool
	return i.base.Execute(ctx, args)
}

// matchesOverride checks if this call matches the override criteria
func (i *InjectionInterceptor) matchesOverride(override ToolResponseOverride, args map[string]interface{}, callNum int) bool {
	// Check call count if specified (0 means all calls)
	if override.CallCount > 0 && callNum != override.CallCount {
		return false
	}

	// Check argument matching if specified
	if len(override.MatchArgs) > 0 {
		for key, expectedVal := range override.MatchArgs {
			actualVal, ok := args[key]
			if !ok {
				return false
			}
			if fmt.Sprint(actualVal) != fmt.Sprint(expectedVal) {
				return false
			}
		}
	}

	return true
}

// createErrorResult creates a ToolResult representing an error
func (i *InjectionInterceptor) createErrorResult(override ToolResponseOverride, msg string) *core.ToolResult {
	return &core.ToolResult{
		Success: false,
		Error:   msg,
		Data: map[string]interface{}{
			"injected": true,
			"tool":     i.base.Name(),
		},
	}
}

// GetCallCount returns how many times the tool has been called
func (i *InjectionInterceptor) GetCallCount() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.callHistory[i.base.Name()]
}

// filterOverridesForTool returns only overrides applicable to this tool
func filterOverridesForTool(overrides []ToolResponseOverride, toolName string) []ToolResponseOverride {
	var filtered []ToolResponseOverride
	for _, o := range overrides {
		if strings.EqualFold(strings.TrimSpace(o.Tool), strings.TrimSpace(toolName)) {
			filtered = append(filtered, o)
		}
	}
	return filtered
}

// WrapRegistryWithInterceptor wraps all tools in the registry with injection support
func WrapRegistryWithInterceptor(registry *capability.Registry, overrides []ToolResponseOverride) *capability.Registry {
	if registry == nil || len(overrides) == 0 {
		return registry
	}

	// Get all callable tools
	tools := registry.CallableTools()

	// Create a new registry and register wrapped tools
	wrappedRegistry := capability.NewRegistry()

	for _, tool := range tools {
		toolOverrides := filterOverridesForTool(overrides, tool.Name())
		if len(toolOverrides) > 0 {
			// Wrap with interceptor
			wrapped := NewInjectionInterceptor(tool, overrides)
			wrappedRegistry.Register(wrapped)
		} else {
			// Register unmodified
			wrappedRegistry.Register(tool)
		}
	}

	return wrappedRegistry
}

// ToolSuccessRate computes the success rate for a tool from telemetry events
func ToolSuccessRate(events []core.Event, toolName string) (successes, failures int, rate float64) {
	for _, ev := range events {
		if ev.Type != core.EventToolResult {
			continue
		}
		tool, _ := ev.Metadata["tool"].(string)
		if tool != toolName {
			continue
		}
		success, _ := ev.Metadata["success"].(bool)
		if success {
			successes++
		} else {
			failures++
		}
	}
	total := successes + failures
	if total > 0 {
		rate = float64(successes) / float64(total)
	}
	return successes, failures, rate
}

// HasRecoveryFromToolFailure checks if agent recovered from any tool failure
func HasRecoveryFromToolFailure(events []core.Event) bool {
	var hadFailure bool
	var hadSuccessAfterFailure bool

	for _, ev := range events {
		switch ev.Type {
		case core.EventToolResult:
			success, _ := ev.Metadata["success"].(bool)
			if !success {
				hadFailure = true
			} else if hadFailure {
				hadSuccessAfterFailure = true
			}
		case core.EventToolCall:
			// Continue checking after failures
		}
	}

	return hadFailure && hadSuccessAfterFailure
}
