package core

import (
	"strings"
	"time"
)

type ContextItem interface {
	Derivation() *DerivationChain
	WithDerivation(chain DerivationChain) ContextItem
	Compress() (ContextItem, error)
}

type MemoryContextItem struct {
	Source          string
	Content         string
	Summary         string
	DerivationChain *DerivationChain
}

func (m *MemoryContextItem) Derivation() *DerivationChain {
	if m == nil {
		return nil
	}
	return m.DerivationChain
}

func (m *MemoryContextItem) WithDerivation(chain DerivationChain) ContextItem {
	if m == nil {
		return nil
	}
	clone := *m
	clone.DerivationChain = &chain
	return &clone
}

func (m *MemoryContextItem) Compress() (ContextItem, error) {
	if m == nil {
		return nil, nil
	}
	clone := *m
	clone.Summary = truncateParagraph(firstNonEmpty(m.Summary, m.Content), 240)
	clone.Content = truncateParagraph(m.Content, 240)
	chain := cloneDerivationChain(clone.DerivationChain)
	derived := chain.Derive("compress_truncate", m.Source, 0.25, clone.Summary)
	clone.DerivationChain = &derived
	return &clone, nil
}

type RetrievalContextItem struct {
	Source          string
	Content         string
	Summary         string
	Reference       any
	LastAccessed    time.Time
	Relevance       float64
	PriorityVal     int
	DerivationChain *DerivationChain
}

func (r *RetrievalContextItem) Derivation() *DerivationChain {
	if r == nil {
		return nil
	}
	return r.DerivationChain
}

func (r *RetrievalContextItem) WithDerivation(chain DerivationChain) ContextItem {
	if r == nil {
		return nil
	}
	clone := *r
	clone.DerivationChain = &chain
	return &clone
}

func (r *RetrievalContextItem) Compress() (ContextItem, error) {
	if r == nil {
		return nil, nil
	}
	clone := *r
	clone.Summary = truncateParagraph(firstNonEmpty(r.Summary, r.Content), 240)
	clone.Content = truncateParagraph(r.Content, 240)
	chain := cloneDerivationChain(clone.DerivationChain)
	derived := chain.Derive("compress_truncate", r.Source, 0.25, clone.Summary)
	clone.DerivationChain = &derived
	return &clone, nil
}

type CapabilityResultContextItem struct {
	ToolName        string
	Result          *ToolResult
	DerivationChain *DerivationChain
}

func (c *CapabilityResultContextItem) Derivation() *DerivationChain {
	if c == nil {
		return nil
	}
	return c.DerivationChain
}

func (c *CapabilityResultContextItem) WithDerivation(chain DerivationChain) ContextItem {
	if c == nil {
		return nil
	}
	clone := *c
	clone.DerivationChain = &chain
	return &clone
}

func (c *CapabilityResultContextItem) Compress() (ContextItem, error) {
	if c == nil {
		return nil, nil
	}
	clone := *c
	if clone.Result != nil {
		if summary := strings.TrimSpace(resultSummaryText(clone.Result)); summary != "" {
			clone.Result = &ToolResult{
				Success:  clone.Result.Success,
				Data:     map[string]any{"summary": truncateParagraph(summary, 240)},
				Error:    clone.Result.Error,
				Metadata: cloneResultMetadata(clone.Result.Metadata),
			}
		}
	}
	chain := cloneDerivationChain(clone.DerivationChain)
	derived := chain.Derive("compress_summarize", c.ToolName, 0.15, truncateParagraph(resultSummaryText(c.Result), 240))
	clone.DerivationChain = &derived
	return &clone, nil
}

func cloneDerivationChain(chain *DerivationChain) DerivationChain {
	if chain != nil && !chain.IsEmpty() {
		return chain.Clone()
	}
	return DerivationChain{}
}

func resultSummaryText(result *ToolResult) string {
	if result == nil {
		return ""
	}
	if summary, ok := result.Data["summary"].(string); ok && strings.TrimSpace(summary) != "" {
		return summary
	}
	if output, ok := result.Data["output"].(string); ok && strings.TrimSpace(output) != "" {
		return output
	}
	return strings.TrimSpace(result.Error)
}

func cloneResultMetadata(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return nil
	}
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
