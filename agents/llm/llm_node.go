// Package llm provides LLM node implementations for agent paradigms.
// This package is shared across different agent implementations.
package llm

import (
	"context"
	"errors"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/compiler"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

// NodeTypeLLM represents an LLM call node type.
const NodeTypeLLM agentgraph.NodeType = "llm"

// LLMNode represents an LLM call. It is a thin wrapper around a LanguageModel
// implementation so that planners can mix LLM "thinking" nodes with tool calls
// or conditional branches inside the same graph.
type LLMNode struct {
	id                 string
	Model              core.LanguageModel
	Prompt             string
	Options            *core.LLMOptions
	CompilationTrigger agentgraph.CompilationTrigger // Optional: triggers context compilation before LLM call
	Query              string                        // Query for context compilation (if CompilationTrigger set)
	MaxTokens          int                           // Max tokens for compiled context
}

// NewLLMNode creates a new LLM node.
func NewLLMNode(id string, model core.LanguageModel, prompt string, options *core.LLMOptions) *LLMNode {
	return &LLMNode{
		id:      id,
		Model:   model,
		Prompt:  prompt,
		Options: options,
	}
}

// SetCompilationTrigger sets the optional compilation trigger for context assembly.
func (n *LLMNode) SetCompilationTrigger(trigger agentgraph.CompilationTrigger, query string, maxTokens int) {
	n.CompilationTrigger = trigger
	n.Query = query
	n.MaxTokens = maxTokens
}

// ID implements agentgraph.Node.
func (n *LLMNode) ID() string { return n.id }

// Type implements agentgraph.Node.
func (n *LLMNode) Type() agentgraph.NodeType { return NodeTypeLLM }

// Contract describes the execution semantics for LLM inference nodes.
func (n *LLMNode) Contract() agentgraph.NodeContract {
	return agentgraph.NodeContract{
		SideEffectClass: agentgraph.SideEffectNone,
		Idempotency:     agentgraph.IdempotencyReplaySafe,
		ContextPolicy: core.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "llm.*"},
			WriteKeys:                []string{"llm.*"},
			AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassWorking},
			AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassStructuredState},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 16,
		},
	}
}

// Execute runs the prompt against the language model.
// If CompilationTrigger is set, context is compiled and added to the prompt.
func (n *LLMNode) Execute(ctx context.Context, state *contextdata.Envelope) (*agentgraph.Result, error) {
	if n.Model == nil {
		return nil, errors.New("llm node missing model")
	}

	// Build the prompt with optional compiled context
	prompt := n.Prompt
	var compiledChunks int
	var compiledTokens int
	var compilationShortfall int

	if n.CompilationTrigger != nil {
		result, _, err := n.CompilationTrigger.Compile(ctx, compiler.CompilationRequest{
			Query:     retrieval.RetrievalQuery{Text: n.Query},
			MaxTokens: n.MaxTokens,
		})
		if err == nil && result != nil {
			// Append compiled context to prompt
			contextText := n.formatCompiledContext(result)
			if contextText != "" {
				prompt = fmt.Sprintf("Context:\n%s\n\n%s", contextText, n.Prompt)
			}
			compiledChunks = len(result.Chunks)
			compiledTokens = result.TotalTokens
			compilationShortfall = result.ShortfallTokens
		}
	}

	resp, err := n.Model.Generate(ctx, prompt, n.Options)
	if err != nil {
		return nil, err
	}

	// Build result data
	data := map[string]interface{}{
		"text": resp.Text,
	}
	if n.CompilationTrigger != nil {
		data["compiled_chunks"] = compiledChunks
		data["compiled_tokens"] = compiledTokens
		data["compilation_shortfall"] = compilationShortfall
	}

	return &agentgraph.Result{
		NodeID:  n.id,
		Success: true,
		Data:    data,
	}, nil
}

// formatCompiledContext converts compiled chunks to a string for the LLM prompt.
func (n *LLMNode) formatCompiledContext(result *compiler.CompilationResult) string {
	if len(result.Chunks) == 0 {
		return ""
	}
	var contextText string
	for _, chunk := range result.Chunks {
		if content, ok := chunk.Body.Fields["content"]; ok {
			contextText += fmt.Sprintf("[%s] %v\n\n", chunk.ID, content)
		}
	}
	return contextText
}
