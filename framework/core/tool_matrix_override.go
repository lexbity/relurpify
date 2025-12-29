package core

// ToolMatrixOverride selectively overrides tool matrix booleans.
type ToolMatrixOverride struct {
	FileRead       *bool `yaml:"file_read,omitempty" json:"file_read,omitempty"`
	FileWrite      *bool `yaml:"file_write,omitempty" json:"file_write,omitempty"`
	FileEdit       *bool `yaml:"file_edit,omitempty" json:"file_edit,omitempty"`
	BashExecute    *bool `yaml:"bash_execute,omitempty" json:"bash_execute,omitempty"`
	LSPQuery       *bool `yaml:"lsp_query,omitempty" json:"lsp_query,omitempty"`
	SearchCodebase *bool `yaml:"search_codebase,omitempty" json:"search_codebase,omitempty"`
	WebSearch      *bool `yaml:"web_search,omitempty" json:"web_search,omitempty"`
}
