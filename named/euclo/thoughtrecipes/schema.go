package thoughtrecipes

// ThoughtRecipe is the top-level type parsed from a thought recipe YAML file.
type ThoughtRecipe struct {
	APIVersion string         `yaml:"apiVersion"` // must be "euclo/v1alpha1"
	Kind       string         `yaml:"kind"`       // must be "ThoughtRecipe"
	Metadata   RecipeMetadata `yaml:"metadata"`
	Global     RecipeGlobal   `yaml:"global"`
	Sequence   []RecipeStep   `yaml:"sequence"`
}

type RecipeMetadata struct {
	Name        string `yaml:"name"`        // required; unique; used as capability ID prefix
	Description string `yaml:"description"` // required
	Version     string `yaml:"version"`     // optional
}

type RecipeGlobal struct {
	Capabilities  RecipeCapabilitySpec `yaml:"capabilities"`
	Context       RecipeContextSpec    `yaml:"context"`
	Configuration RecipeConfiguration  `yaml:"configuration"`
	Prompt        string               `yaml:"prompt"`
}

type RecipeCapabilitySpec struct {
	Allowed []string `yaml:"allowed"` // nil = inherit from parent scope / manifest
}

type RecipeContextSpec struct {
	Enrichment []string          `yaml:"enrichment"` // ast | archaeology | bkc
	Sharing    RecipeSharingSpec `yaml:"sharing"`
	Aliases    map[string]string `yaml:"aliases"` // alias name → state key
}

type RecipeSharingSpec struct {
	Default string `yaml:"default"` // carry_forward | isolated | explicit
}

type RecipeConfiguration struct {
	Modes                  []string `yaml:"modes"`
	IntentKeywords         []string `yaml:"intent_keywords"`
	TriggerPriority        int      `yaml:"trigger_priority"`
	AllowDynamicResolution bool     `yaml:"allow_dynamic_resolution"`
}

type RecipeStep struct {
	ID       string           `yaml:"id"`
	Parent   RecipeStepAgent  `yaml:"parent"`
	Child    *RecipeStepAgent `yaml:"child,omitempty"`
	Fallback *RecipeStepAgent `yaml:"fallback,omitempty"`
}

type RecipeStepAgent struct {
	Paradigm     string               `yaml:"paradigm"`
	Prompt       string               `yaml:"prompt"`
	Context      RecipeStepContext    `yaml:"context"`
	Capabilities RecipeCapabilitySpec `yaml:"capabilities"`
}

type RecipeStepContext struct {
	Enrichment []string `yaml:"enrichment"` // additive to global
	Inherit    []string `yaml:"inherit"`    // alias names to inject from prior steps
	Capture    []string `yaml:"capture"`    // alias names this agent writes
}
