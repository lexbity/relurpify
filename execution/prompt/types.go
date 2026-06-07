package prompt

// PromptConfig is the parsed, immutable representation of a v2 prompt asset.
type PromptConfig struct {
	Schema     string
	ID         string
	Tags       []string
	Variables  map[string]VariableDecl
	Body       string
	SourcePath string
}

// VariableDecl declares a prompt variable with an optional default value.
type VariableDecl struct {
	Default string
}
