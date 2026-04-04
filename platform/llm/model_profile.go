package llm

import "gopkg.in/yaml.v3"

// ModelProfile captures model-specific quirks and configuration.
type ModelProfile struct {
	// Pattern is matched against the model name. Supports prefix matching
	// and glob wildcards (e.g. "qwen2.5-coder*", "llama3*", "*").
	Pattern string `yaml:"pattern"`

	ToolCalling struct {
		// NativeAPI maps to core.Config.OllamaToolCalling when not explicitly set.
		NativeAPI bool `yaml:"native_api"`
		// DoubleEncodedArgs enables double-decode in parseArguments (qwen quirk:
		// arguments JSON is sometimes returned as a quoted string).
		DoubleEncodedArgs bool `yaml:"double_encoded_args"`
		// MultilineStringLiterals enables normalizeMultilineJSONStringLiterals
		// (qwen quirk: literal newlines inside JSON string values).
		MultilineStringLiterals bool `yaml:"multiline_string_literals"`
		// MaxToolsPerCall limits how many tool calls are processed per response.
		// 0 = no limit.
		MaxToolsPerCall int `yaml:"max_tools_per_call"`
	} `yaml:"tool_calling"`

	Repair struct {
		// Strategy controls repair behaviour when tool call parsing fails.
		// "llm"            — issue a second Generate call with a repair prompt.
		// "heuristic-only" — use text heuristics only, no second LLM call.
		Strategy    string `yaml:"strategy"`
		MaxAttempts int    `yaml:"max_attempts"`
	} `yaml:"repair"`

	Schema struct {
		// FlattenNested collapses nested object schemas to top-level properties
		// for models that cannot handle nested parameter schemas.
		FlattenNested bool `yaml:"flatten_nested"`
		// MaxDescriptionLen truncates tool/parameter descriptions to this length.
		// 0 = no truncation.
		MaxDescriptionLen int `yaml:"max_description_len"`
	} `yaml:"schema"`
}
