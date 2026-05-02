package prompt

// PromptConfig is the parsed, immutable representation of a .prompt file.
// SourcePath and ParentResolved are populated by registry indexing, not the parser.
type PromptConfig struct {
	APIVersion        string
	ID                string
	Name              string
	Description       string
	Extends           string
	FrameworkCritical bool
	RequiresProviders []string
	Tags              Tags
	Variables         map[string]VariableDecl
	Blocks            []PromptBlock

	SourcePath     string
	ParentResolved *PromptConfig
}

// Tags categorises a prompt along five orthogonal axes.
type Tags struct {
	Paradigm  []string
	Agent     []string
	Domain    []string
	Kind      string
	Stability string
}

// VariableDecl declares a prompt variable with an optional default value.
type VariableDecl struct {
	Default string
}

// PromptBlock is a single assembled section within a prompt.
type PromptBlock struct {
	ID       string
	Name     string
	Kind     string
	Order    int
	When     Expression // nil when no condition
	From     BlockSource
	Provider string // empty when From == SourceStatic
	Locked   bool
	Content  string // raw prose pre-interpolation; empty for SourceProvider blocks
}

// BlockSource distinguishes static content from provider-supplied content.
type BlockSource int

const (
	SourceStatic   BlockSource = iota
	SourceProvider             // from: provider
)

// Expression is a compiled when-expression that can be evaluated against
// runtime state. The concrete implementation lives in the parser sub-package.
// A nil Expression means the block is unconditionally included.
type Expression interface {
	// Evaluate returns true if the block should be included.
	// A malformed evaluation must return (false, non-nil error).
	Evaluate(state map[string]any) (bool, error)
}

// Named order positions and their integer equivalents.
const (
	OrderEarly  = 10
	OrderMiddle = 50
	OrderLate   = 80
	OrderLast   = 99
)

// ParseNamedOrder converts a named order string ("early", "middle", "late",
// "last") to its integer equivalent. Returns 0, false for unknown names.
func ParseNamedOrder(s string) (int, bool) {
	switch s {
	case "early":
		return OrderEarly, true
	case "middle":
		return OrderMiddle, true
	case "late":
		return OrderLate, true
	case "last":
		return OrderLast, true
	default:
		return 0, false
	}
}
