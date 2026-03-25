package core

import (
	"fmt"
	"time"
)

type ContextReferenceKind string

const (
	ContextReferenceFile              ContextReferenceKind = "file"
	ContextReferenceRetrievalEvidence ContextReferenceKind = "retrieval_evidence"
	ContextReferenceRuntimeMemory     ContextReferenceKind = "runtime_memory"
	ContextReferenceWorkflowArtifact  ContextReferenceKind = "workflow_artifact"
)

type ContextReference struct {
	Kind     ContextReferenceKind `json:"kind" yaml:"kind"`
	ID       string               `json:"id,omitempty" yaml:"id,omitempty"`
	URI      string               `json:"uri,omitempty" yaml:"uri,omitempty"`
	Version  string               `json:"version,omitempty" yaml:"version,omitempty"`
	Detail   string               `json:"detail,omitempty" yaml:"detail,omitempty"`
	Metadata map[string]string    `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// ContextItem represents a unit that can be managed for budget purposes.
type ContextItem interface {
	TokenCount() int
	RelevanceScore() float64
	Priority() int
	Compress() (ContextItem, error)
	Type() ContextItemType
	Age() time.Duration
}

// ReferenceCapableContextItem exposes a stable reference that callers can keep
// even when inline payload is compressed or omitted.
type ReferenceCapableContextItem interface {
	ContextItem
	References() []ContextReference
	HasInlinePayload() bool
}

// DerivationCapableContextItem exposes the transformation history of a context item.
type DerivationCapableContextItem interface {
	ContextItem
	Derivation() *DerivationChain
	WithDerivation(chain DerivationChain) ContextItem
}

// ContextItemType categorizes managed items.
type ContextItemType string

const (
	ContextTypeInteraction      ContextItemType = "interaction"
	ContextTypeFile             ContextItemType = "file"
	ContextTypeRetrieval        ContextItemType = "retrieval"
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
	Path              string
	Content           string
	Summary           string
	Reference         *ContextReference
	LastAccessed      time.Time
	Relevance         float64
	PriorityVal       int
	Pinned            bool
	derives   *DerivationChain
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

	// Calculate compression loss
	lossMagnitude := 0.0
	if fci.Content != "" {
		lossMagnitude = float64(len(fci.Content)-len(summary)) / float64(len(fci.Content))
		if lossMagnitude > 0.7 {
			lossMagnitude = 0.7
		}
	}

	chain := fci.derivationOrEmpty()
	if lossMagnitude > 0 {
		chain = chain.Derive("compress_truncate", "contextmgr", lossMagnitude, "")
	}

	return &FileContextItem{
		Path:         fci.Path,
		Content:      "",
		Summary:      summary,
		Reference:    cloneContextReference(fci.Reference),
		LastAccessed: fci.LastAccessed,
		Relevance:    fci.Relevance * 0.9,
		PriorityVal:  fci.PriorityVal + 1,
		Pinned:       fci.Pinned,
		derives:   &chain,
	}, nil
}

func (fci *FileContextItem) Type() ContextItemType {
	return ContextTypeFile
}

func (fci *FileContextItem) Age() time.Duration {
	return time.Since(fci.LastAccessed)
}

func (fci *FileContextItem) References() []ContextReference {
	if fci == nil || fci.Reference == nil {
		return nil
	}
	return []ContextReference{*cloneContextReference(fci.Reference)}
}

func (fci *FileContextItem) HasInlinePayload() bool {
	return fci != nil && (fci.Content != "" || fci.Summary != "")
}

func (fci *FileContextItem) Derivation() *DerivationChain {
	return fci.derives
}

func (fci *FileContextItem) WithDerivation(chain DerivationChain) ContextItem {
	copied := *fci
	copied.derives = &chain
	return &copied
}

func (fci *FileContextItem) derivationOrEmpty() DerivationChain {
	if fci.derives == nil {
		return DerivationChain{Steps: []DerivationStep{}}
	}
	return fci.derives.Clone()
}

// MemoryContextItem captures retrieved memories as a synthetic context block.
type MemoryContextItem struct {
	Source       string
	Content      string
	Summary      string
	Reference    *ContextReference
	LastAccessed time.Time
	Relevance    float64
	PriorityVal  int
	derives      *DerivationChain
}

func (mci *MemoryContextItem) TokenCount() int {
	if mci.Content != "" {
		return estimateTokens(mci.Content)
	}
	if mci.Summary != "" {
		return estimateTokens(mci.Summary)
	}
	return estimateTokens(mci.Source)
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
	summary := mci.Summary
	if summary == "" {
		summary = content
	}

	// Calculate compression loss based on truncation
	lossMagnitude := 0.0
	if mci.Content != "" && content != mci.Content {
		lossMagnitude = float64(len(mci.Content)-len(content)) / float64(len(mci.Content))
		if lossMagnitude > 0.6 {
			lossMagnitude = 0.6
		}
	}

	chain := mci.derivationOrEmpty()
	if lossMagnitude > 0 {
		chain = chain.Derive("compress_truncate", "contextmgr", lossMagnitude, "")
	}

	return &MemoryContextItem{
		Source:       mci.Source,
		Content:      "",
		Summary:      summary,
		Reference:    cloneContextReference(mci.Reference),
		LastAccessed: mci.LastAccessed,
		Relevance:    mci.Relevance * 0.9,
		PriorityVal:  mci.PriorityVal + 1,
		derives:   &chain,
	}, nil
}

func (mci *MemoryContextItem) Type() ContextItemType {
	return ContextTypeMemory
}

func (mci *MemoryContextItem) Age() time.Duration {
	return time.Since(mci.LastAccessed)
}

func (mci *MemoryContextItem) References() []ContextReference {
	if mci == nil || mci.Reference == nil {
		return nil
	}
	return []ContextReference{*cloneContextReference(mci.Reference)}
}

func (mci *MemoryContextItem) HasInlinePayload() bool {
	return mci != nil && (mci.Content != "" || mci.Summary != "")
}

func (mci *MemoryContextItem) Derivation() *DerivationChain {
	return mci.derives
}

func (mci *MemoryContextItem) WithDerivation(chain DerivationChain) ContextItem {
	copied := *mci
	copied.derives = &chain
	return &copied
}

func (mci *MemoryContextItem) derivationOrEmpty() DerivationChain {
	if mci.derives == nil {
		return DerivationChain{Steps: []DerivationStep{}}
	}
	return mci.derives.Clone()
}

// RetrievalContextItem captures packed retrieval evidence with a stable
// reference so prompt assembly can choose between inline hydration and
// reference-only retention.
type RetrievalContextItem struct {
	Source       string
	Content      string
	Summary      string
	Reference    *ContextReference
	LastAccessed time.Time
	Relevance    float64
	PriorityVal  int
	derives      *DerivationChain
}

func (rci *RetrievalContextItem) TokenCount() int {
	if rci.Content != "" {
		return estimateTokens(rci.Content)
	}
	if rci.Summary != "" {
		return estimateTokens(rci.Summary)
	}
	return estimateTokens(rci.Source)
}

func (rci *RetrievalContextItem) RelevanceScore() float64 {
	if rci.Relevance == 0 {
		rci.Relevance = 0.85
	}
	age := time.Since(rci.LastAccessed)
	decay := 1.0 / (1.0 + age.Hours()/12.0)
	return rci.Relevance * decay
}

func (rci *RetrievalContextItem) Priority() int {
	return rci.PriorityVal
}

func (rci *RetrievalContextItem) Compress() (ContextItem, error) {
	summary := rci.Summary
	if summary == "" {
		summary = truncate(rci.Content, 250)
	}

	// Calculate compression loss
	lossMagnitude := 0.0
	if rci.Content != "" {
		lossMagnitude = float64(len(rci.Content)-len(summary)) / float64(len(rci.Content))
		if lossMagnitude > 0.6 {
			lossMagnitude = 0.6
		}
	}

	chain := rci.derivationOrEmpty()
	if lossMagnitude > 0 {
		chain = chain.Derive("compress_truncate", "contextmgr", lossMagnitude, "")
	}

	return &RetrievalContextItem{
		Source:       rci.Source,
		Content:      "",
		Summary:      summary,
		Reference:    cloneContextReference(rci.Reference),
		LastAccessed: rci.LastAccessed,
		Relevance:    rci.Relevance * 0.9,
		PriorityVal:  rci.PriorityVal + 1,
		derives:   &chain,
	}, nil
}

func (rci *RetrievalContextItem) Type() ContextItemType {
	return ContextTypeRetrieval
}

func (rci *RetrievalContextItem) Age() time.Duration {
	return time.Since(rci.LastAccessed)
}

func (rci *RetrievalContextItem) References() []ContextReference {
	if rci == nil || rci.Reference == nil {
		return nil
	}
	return []ContextReference{*cloneContextReference(rci.Reference)}
}

func (rci *RetrievalContextItem) HasInlinePayload() bool {
	return rci != nil && (rci.Content != "" || rci.Summary != "")
}

func (rci *RetrievalContextItem) Derivation() *DerivationChain {
	return rci.derives
}

func (rci *RetrievalContextItem) WithDerivation(chain DerivationChain) ContextItem {
	copied := *rci
	copied.derives = &chain
	return &copied
}

func (rci *RetrievalContextItem) derivationOrEmpty() DerivationChain {
	if rci.derives == nil {
		return DerivationChain{Steps: []DerivationStep{}}
	}
	return rci.derives.Clone()
}

// CapabilityResultContextItem represents structured capability outputs inside context.
type CapabilityResultContextItem struct {
	ToolName     string
	Result       *ToolResult
	Envelope     *CapabilityResultEnvelope
	LastAccessed time.Time
	Relevance    float64
	PriorityVal  int
	derives      *DerivationChain
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
	originalLen := len(payload)
	if len(payload) > 250 {
		payload = payload[:250] + "..."
	}

	// Calculate compression loss based on summarization
	lossMagnitude := 0.45 // Standard loss for summarization
	if originalLen <= 250 {
		lossMagnitude = 0.0 // No loss if already under limit
	}

	chain := tr.derivationOrEmpty()
	if lossMagnitude > 0 {
		chain = chain.Derive("compress_summarize", "contextmgr", lossMagnitude, "")
	}

	return &CapabilityResultContextItem{
		ToolName:     tr.ToolName,
		Result:       &ToolResult{Success: tr.Result.Success, Data: map[string]interface{}{"summary": payload}},
		Envelope:     SummarizeCapabilityResultEnvelope(tr.Envelope, payload),
		LastAccessed: tr.LastAccessed,
		Relevance:    tr.Relevance * 0.9,
		PriorityVal:  tr.PriorityVal + 1,
		derives:   &chain,
	}, nil
}

func (tr *CapabilityResultContextItem) Type() ContextItemType {
	return ContextTypeCapabilityResult
}

func (tr *CapabilityResultContextItem) Age() time.Duration {
	return time.Since(tr.LastAccessed)
}

func (tr *CapabilityResultContextItem) Derivation() *DerivationChain {
	return tr.derives
}

func (tr *CapabilityResultContextItem) WithDerivation(chain DerivationChain) ContextItem {
	copied := *tr
	copied.derives = &chain
	return &copied
}

func (tr *CapabilityResultContextItem) derivationOrEmpty() DerivationChain {
	if tr.derives == nil {
		return DerivationChain{Steps: []DerivationStep{}}
	}
	return tr.derives.Clone()
}

func cloneContextReference(ref *ContextReference) *ContextReference {
	if ref == nil {
		return nil
	}
	out := *ref
	if len(ref.Metadata) > 0 {
		out.Metadata = make(map[string]string, len(ref.Metadata))
		for key, value := range ref.Metadata {
			out.Metadata[key] = value
		}
	}
	return &out
}

// ToolResultContextItem is a compatibility alias retained while tool-specific
// execution paths migrate to capability-native terminology.
type ToolResultContextItem = CapabilityResultContextItem
