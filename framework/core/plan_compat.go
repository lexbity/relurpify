package core

// PlanStep is the flat execution-plan step used by graph/HTN/ReWoo-style agents.
// The richer living-plan model lives in framework/plan.
type PlanStep struct {
	ID                string         `json:"id,omitempty" yaml:"id,omitempty"`
	Description       string         `json:"description,omitempty" yaml:"description,omitempty"`
	Tool              string         `json:"tool,omitempty" yaml:"tool,omitempty"`
	Params            map[string]any `json:"params,omitempty" yaml:"params,omitempty"`
	DependsOn         []string       `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Expected          string         `json:"expected,omitempty" yaml:"expected,omitempty"`
	Verification      string         `json:"verification,omitempty" yaml:"verification,omitempty"`
	Files             []string       `json:"files,omitempty" yaml:"files,omitempty"`
	ContinueOnFailure bool           `json:"continue_on_failure,omitempty" yaml:"continue_on_failure,omitempty"`
}

// Plan is the flat workflow plan used by many agents.
type Plan struct {
	Goal         string              `json:"goal,omitempty" yaml:"goal,omitempty"`
	Steps        []PlanStep          `json:"steps,omitempty" yaml:"steps,omitempty"`
	Dependencies map[string][]string `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	Files        []string            `json:"files,omitempty" yaml:"files,omitempty"`
}
