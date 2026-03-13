package core

import (
	"fmt"
	"time"
)

// ContextItem represents a unit that can be managed for budget purposes.
type ContextItem interface {
	TokenCount() int
	RelevanceScore() float64
	Priority() int
	Compress() (ContextItem, error)
	Type() ContextItemType
	Age() time.Duration
}

// ContextItemType categorizes managed items.
type ContextItemType string

const (
	ContextTypeInteraction      ContextItemType = "interaction"
	ContextTypeFile             ContextItemType = "file"
	ContextTypeCapabilityResult ContextItemType = "capability_result"
	ContextTypeToolResult       ContextItemType = ContextTypeCapabilityResult
	ContextTypeMemory           ContextItemType = "memory"
	ContextTypeObservation      ContextItemType = "observation"
)

// InteractionContextItem wraps an Interaction as a context item.
type InteractionContextItem struct {
	Interaction Interaction
	Relevance   float64
	PriorityVal int
}

func (ici *InteractionContextItem) TokenCount() int {
	return len(ici.Interaction.Content) / 4
}

func (ici *InteractionContextItem) RelevanceScore() float64 {
	age := time.Since(ici.Interaction.Timestamp)
	decay := 1.0 / (1.0 + age.Hours()/24.0)
	return ici.Relevance * decay
}

func (ici *InteractionContextItem) Priority() int {
	return ici.PriorityVal
}

func (ici *InteractionContextItem) Compress() (ContextItem, error) {
	return &InteractionContextItem{
		Interaction: Interaction{
			ID:        ici.Interaction.ID,
			Role:      ici.Interaction.Role,
			Content:   truncate(ici.Interaction.Content, 100),
			Timestamp: ici.Interaction.Timestamp,
			Metadata:  ici.Interaction.Metadata,
		},
		Relevance:   ici.Relevance * 0.8,
		PriorityVal: ici.PriorityVal + 1,
	}, nil
}

func (ici *InteractionContextItem) Type() ContextItemType {
	return ContextTypeInteraction
}

func (ici *InteractionContextItem) Age() time.Duration {
	return time.Since(ici.Interaction.Timestamp)
}

// FileContextItem represents file contents tracked in context.
type FileContextItem struct {
	Path         string
	Content      string
	Summary      string
	LastAccessed time.Time
	Relevance    float64
	PriorityVal  int
	Pinned       bool
}

func (fci *FileContextItem) TokenCount() int {
	data := fci.Content
	if data == "" {
		data = fci.Summary
	}
	return len(data) / 4
}

func (fci *FileContextItem) RelevanceScore() float64 {
	if fci.Pinned {
		return 1.0
	}
	age := time.Since(fci.LastAccessed)
	decay := 1.0 / (1.0 + age.Minutes()/60.0)
	return fci.Relevance * decay
}

func (fci *FileContextItem) Priority() int {
	if fci.Pinned {
		return 0
	}
	return fci.PriorityVal
}

func (fci *FileContextItem) Compress() (ContextItem, error) {
	summary := fci.Summary
	if summary == "" {
		summary = truncate(fci.Content, 200)
	}
	return &FileContextItem{
		Path:         fci.Path,
		Content:      "",
		Summary:      summary,
		LastAccessed: fci.LastAccessed,
		Relevance:    fci.Relevance * 0.9,
		PriorityVal:  fci.PriorityVal + 1,
		Pinned:       fci.Pinned,
	}, nil
}

func (fci *FileContextItem) Type() ContextItemType {
	return ContextTypeFile
}

func (fci *FileContextItem) Age() time.Duration {
	return time.Since(fci.LastAccessed)
}

// MemoryContextItem captures retrieved memories as a synthetic context block.
type MemoryContextItem struct {
	Source       string
	Content      string
	LastAccessed time.Time
	Relevance    float64
	PriorityVal  int
}

func (mci *MemoryContextItem) TokenCount() int {
	return estimateTokens(mci.Content)
}

func (mci *MemoryContextItem) RelevanceScore() float64 {
	if mci.Relevance == 0 {
		mci.Relevance = 0.85
	}
	age := time.Since(mci.LastAccessed)
	decay := 1.0 / (1.0 + age.Hours()/12.0)
	return mci.Relevance * decay
}

func (mci *MemoryContextItem) Priority() int {
	return mci.PriorityVal
}

func (mci *MemoryContextItem) Compress() (ContextItem, error) {
	content := mci.Content
	if len(content) > 250 {
		content = content[:250] + "..."
	}
	return &MemoryContextItem{
		Source:       mci.Source,
		Content:      content,
		LastAccessed: mci.LastAccessed,
		Relevance:    mci.Relevance * 0.9,
		PriorityVal:  mci.PriorityVal + 1,
	}, nil
}

func (mci *MemoryContextItem) Type() ContextItemType {
	return ContextTypeMemory
}

func (mci *MemoryContextItem) Age() time.Duration {
	return time.Since(mci.LastAccessed)
}

// CapabilityResultContextItem represents structured capability outputs inside context.
type CapabilityResultContextItem struct {
	ToolName     string
	Result       *ToolResult
	Envelope     *CapabilityResultEnvelope
	LastAccessed time.Time
	Relevance    float64
	PriorityVal  int
}

func (tr *CapabilityResultContextItem) tokenPayload() string {
	if tr != nil && tr.Envelope != nil && tr.Envelope.Result != nil {
		if data := tr.Envelope.Result.Data; len(data) > 0 {
			return fmt.Sprintf("%v", data)
		}
		if msg := tr.Envelope.Result.Error; msg != "" {
			return msg
		}
	}
	if tr == nil || tr.Result == nil {
		return ""
	}
	if len(tr.Result.Data) == 0 {
		return tr.Result.Error
	}
	return fmt.Sprintf("%v", tr.Result.Data)
}

func (tr *CapabilityResultContextItem) TokenCount() int {
	return estimateTokens(tr.tokenPayload())
}

func (tr *CapabilityResultContextItem) RelevanceScore() float64 {
	if tr.Relevance == 0 {
		tr.Relevance = 0.8
	}
	age := time.Since(tr.LastAccessed)
	decay := 1.0 / (1.0 + age.Hours()/12.0)
	return tr.Relevance * decay
}

func (tr *CapabilityResultContextItem) Priority() int {
	return tr.PriorityVal
}

func (tr *CapabilityResultContextItem) Compress() (ContextItem, error) {
	payload := tr.tokenPayload()
	if len(payload) > 250 {
		payload = payload[:250] + "..."
	}
	return &CapabilityResultContextItem{
		ToolName:     tr.ToolName,
		Result:       &ToolResult{Success: tr.Result.Success, Data: map[string]interface{}{"summary": payload}},
		Envelope:     SummarizeCapabilityResultEnvelope(tr.Envelope, payload),
		LastAccessed: tr.LastAccessed,
		Relevance:    tr.Relevance * 0.9,
		PriorityVal:  tr.PriorityVal + 1,
	}, nil
}

func (tr *CapabilityResultContextItem) Type() ContextItemType {
	return ContextTypeCapabilityResult
}

func (tr *CapabilityResultContextItem) Age() time.Duration {
	return time.Since(tr.LastAccessed)
}

// ToolResultContextItem is a compatibility alias retained while tool-specific
// execution paths migrate to capability-native terminology.
type ToolResultContextItem = CapabilityResultContextItem
