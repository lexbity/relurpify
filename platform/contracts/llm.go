package contracts

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/capability/ports"
	"codeburg.org/lexbit/relurpify/framework/capability/schemacoerce"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// UsageObserver is implemented by framework components that want to observe
// token usage after each LLM call. Stored in context by the framework layer;
// retrieved by InstrumentedModel without importing framework packages.
type UsageObserver = telemetry.UsageObserver

// SnapshotObserver is implemented by framework components that want to record
// periodic budget snapshots. Called after every LLM response.
type SnapshotObserver = telemetry.SnapshotObserver

// ResponseIngester is implemented by framework components that want to index
// LLM responses into the knowledge graph as durable chunks.
type ResponseIngester = telemetry.ResponseIngester

type (
	usageObserverKey    struct{}
	snapshotObserverKey struct{}
	responseIngesterKey struct{}
)

// WithUsageObserver attaches a UsageObserver to the context.
func WithUsageObserver(ctx context.Context, obs UsageObserver) context.Context {
	return telemetry.WithUsageObserver(ctx, obs)
}

// UsageObserverFromContext extracts the UsageObserver from context, or nil.
func UsageObserverFromContext(ctx context.Context) UsageObserver {
	return telemetry.UsageObserverFromContext(ctx)
}

// WithSnapshotObserver attaches a SnapshotObserver to the context.
func WithSnapshotObserver(ctx context.Context, obs SnapshotObserver) context.Context {
	return telemetry.WithSnapshotObserver(ctx, obs)
}

// SnapshotObserverFromContext extracts the SnapshotObserver from context, or nil.
func SnapshotObserverFromContext(ctx context.Context) SnapshotObserver {
	return telemetry.SnapshotObserverFromContext(ctx)
}

// WithResponseIngester attaches a ResponseIngester to the context.
func WithResponseIngester(ctx context.Context, ing ResponseIngester) context.Context {
	return telemetry.WithResponseIngester(ctx, ing)
}

// ResponseIngesterFromContext extracts the ResponseIngester from context, or nil.
func ResponseIngesterFromContext(ctx context.Context) ResponseIngester {
	return telemetry.ResponseIngesterFromContext(ctx)
}

// LLMToolSpecFromTool converts a Tool to an LLMToolSpec.
func LLMToolSpecFromTool(t Tool) LLMToolSpec {
	spec := LLMToolSpec{
		Name:        t.Name(),
		Description: t.Description(),
	}
	params := t.Parameters()
	if len(params) > 0 {
		props := make(map[string]*Schema, len(params))
		var required []string
		for _, p := range params {
			prop := &Schema{
				Type:        string(p.Type),
				Description: p.Description,
			}
			if p.Default != nil {
				prop.Default = p.Default
			}
			props[p.Name] = prop
			if p.Required {
				required = append(required, p.Name)
			}
		}
		spec.InputSchema = &Schema{
			Type:       "object",
			Properties: props,
			Required:   required,
		}
	}
	return spec
}

// LLMToolSpecsFromTools converts a slice of Tool to LLMToolSpec values.
func LLMToolSpecsFromTools(tools []Tool) []LLMToolSpec {
	if len(tools) == 0 {
		return nil
	}
	specs := make([]LLMToolSpec, len(tools))
	for i, t := range tools {
		specs[i] = LLMToolSpecFromTool(t)
	}
	return specs
}

type LanguageModel = model.LanguageModel
type LLMOptions = model.LLMOptions
type LLMResponse = model.LLMResponse
type ToolCall = model.ToolCall
type Message = model.Message
type LLMToolSpec = ports.LLMToolSpec
type Schema = schemacoerce.Schema
type BackendClass = model.BackendClass

const (
	BackendClassTransport = model.BackendClassTransport
	BackendClassNative    = model.BackendClassNative
)

type ProfiledModel = model.ProfiledModel
type BackendCapabilities = model.BackendCapabilities
type ModelProfile = model.ModelProfile
type Telemetry = telemetry.Telemetry
type Event = telemetry.Event
type EventType = telemetry.EventType

const (
	EventLLMPrompt            = telemetry.EventLLMPrompt
	EventLLMResponse          = telemetry.EventLLMResponse
	EventBudgetSnapshot       = telemetry.EventBudgetSnapshot
	EventSessionResetRequired = telemetry.EventSessionResetRequired
)

type TokenUsageReport = telemetry.TokenUsage

func EstimateTokens(text string) int { return telemetry.EstimateTokens(text) }