package interaction

// InteractionConfig holds euclo interaction configuration.
// Can be populated from manifest YAML or programmatically.
type InteractionConfig struct {
	Enabled        bool                    `yaml:"enabled" json:"enabled"`
	Budget         InteractionBudgetConfig `yaml:"budget,omitempty" json:"budget,omitempty"`
	Defaults       InteractionDefaults     `yaml:"defaults,omitempty" json:"defaults,omitempty"`
	Classification ClassificationConfig    `yaml:"classification,omitempty" json:"classification,omitempty"`
}

// InteractionBudgetConfig defines budget limits.
type InteractionBudgetConfig struct {
	MaxQuestions   int `yaml:"max_questions,omitempty" json:"max_questions,omitempty"` // per phase
	MaxTransitions int `yaml:"max_transitions,omitempty" json:"max_transitions,omitempty"`
	MaxFrames      int `yaml:"max_frames,omitempty" json:"max_frames,omitempty"`
}

// InteractionDefaults defines default behaviors.
type InteractionDefaults struct {
	SkipConfirmation       bool `yaml:"skip_confirmation,omitempty" json:"skip_confirmation,omitempty"`
	AutoPlanThreshold      int  `yaml:"auto_plan_threshold,omitempty" json:"auto_plan_threshold,omitempty"`           // files count
	VerifyFailureThreshold int  `yaml:"verify_failure_threshold,omitempty" json:"verify_failure_threshold,omitempty"` // failures before debug
}

// ClassificationConfig defines classification behavior.
type ClassificationConfig struct {
	LLMFallback bool `yaml:"llm_fallback,omitempty" json:"llm_fallback,omitempty"` // LLM for ambiguous tasks
}

// DefaultInteractionConfig returns a config with sensible defaults.
func DefaultInteractionConfig() InteractionConfig {
	return InteractionConfig{
		Enabled: true,
		Budget: InteractionBudgetConfig{
			MaxQuestions:   3,
			MaxTransitions: 3,
		},
		Defaults: InteractionDefaults{
			AutoPlanThreshold:      5,
			VerifyFailureThreshold: 2,
		},
	}
}
