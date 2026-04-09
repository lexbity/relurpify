package ollama

// ModelProfile captures model-specific quirks and configuration.
type ModelProfile struct {
	Pattern string `yaml:"pattern"`

	ToolCalling struct {
		NativeAPI               bool `yaml:"native_api"`
		DoubleEncodedArgs       bool `yaml:"double_encoded_args"`
		MultilineStringLiterals bool `yaml:"multiline_string_literals"`
		MaxToolsPerCall         int  `yaml:"max_tools_per_call"`
	} `yaml:"tool_calling"`

	Repair struct {
		Strategy    string `yaml:"strategy"`
		MaxAttempts int    `yaml:"max_attempts"`
	} `yaml:"repair"`

	Schema struct {
		FlattenNested     bool `yaml:"flatten_nested"`
		MaxDescriptionLen int  `yaml:"max_description_len"`
	} `yaml:"schema"`
}
