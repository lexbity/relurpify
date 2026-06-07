package context

type ASTSymbolSummary struct {
	Name       string
	Kind       string
	File       string
	Line       int
	Signature  string
	DocSummary string
}

type AgentSemanticContext struct {
	Chunks          []AgentContextChunk
	ASTSymbols      []ASTSymbolSummary
	TokenBudgetUsed int
	WorkspaceID     string
	WorkflowID      string
	CodeRevision    string
}

type AgentContextChunk struct {
	ID            string
	Content       string
	TokenEstimate int
	Metadata      map[string]string
}

func (e AgentSemanticContext) IsEmpty() bool {
	return len(e.Chunks) == 0 &&
		len(e.ASTSymbols) == 0 &&
		e.TokenBudgetUsed == 0 &&
		e.WorkspaceID == "" &&
		e.WorkflowID == "" &&
		e.CodeRevision == ""
}
