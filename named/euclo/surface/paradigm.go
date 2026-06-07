package surface

// Paradigm identifies an agent or recipe paradigm.
type Paradigm string

const (
	ParadigmReact      Paradigm = "react"
	ParadigmPlanner    Paradigm = "planner"
	ParadigmHTN        Paradigm = "htn"
	ParadigmReflection Paradigm = "reflection"
	ParadigmBlackboard Paradigm = "blackboard"
	ParadigmChainer    Paradigm = "chainer"
	ParadigmPipeline   Paradigm = "pipeline"
	ParadigmRewoo      Paradigm = "rewoo"
	ParadigmGoalcon    Paradigm = "goalcon"
	ParadigmEuclo      Paradigm = "euclo"
)

// AllParadigms returns all known paradigm values.
func AllParadigms() []Paradigm {
	return []Paradigm{
		ParadigmReact,
		ParadigmPlanner,
		ParadigmHTN,
		ParadigmReflection,
		ParadigmBlackboard,
		ParadigmChainer,
		ParadigmPipeline,
		ParadigmRewoo,
		ParadigmGoalcon,
		ParadigmEuclo,
	}
}

// AgentParadigms returns paradigms that are valid for agent dispatch (excludes euclo).
func AgentParadigms() []Paradigm {
	return []Paradigm{
		ParadigmReact,
		ParadigmPlanner,
		ParadigmHTN,
		ParadigmReflection,
		ParadigmBlackboard,
		ParadigmChainer,
		ParadigmPipeline,
		ParadigmRewoo,
		ParadigmGoalcon,
	}
}

// IsSupported reports whether p is a supported agent-dispatchable paradigm.
func IsSupported(p Paradigm) bool {
	switch p {
	case ParadigmReact, ParadigmPlanner, ParadigmHTN,
		ParadigmReflection, ParadigmBlackboard, ParadigmChainer,
		ParadigmPipeline, ParadigmRewoo, ParadigmGoalcon:
		return true
	default:
		return false
	}
}

// ParadigmMeta holds human-readable metadata about a paradigm.
type ParadigmMeta struct {
	Label      string
	ShortGlyph string
	Family     string
}

// Describe returns metadata for the given paradigm.
func (p Paradigm) Describe() ParadigmMeta {
	switch p {
	case ParadigmReact:
		return ParadigmMeta{Label: "React", ShortGlyph: "RX", Family: "agent"}
	case ParadigmPlanner:
		return ParadigmMeta{Label: "Planner", ShortGlyph: "PL", Family: "agent"}
	case ParadigmHTN:
		return ParadigmMeta{Label: "HTN", ShortGlyph: "HT", Family: "agent"}
	case ParadigmReflection:
		return ParadigmMeta{Label: "Reflection", ShortGlyph: "RF", Family: "agent"}
	case ParadigmBlackboard:
		return ParadigmMeta{Label: "Blackboard", ShortGlyph: "BB", Family: "agent"}
	case ParadigmChainer:
		return ParadigmMeta{Label: "Chainer", ShortGlyph: "CH", Family: "agent"}
	case ParadigmPipeline:
		return ParadigmMeta{Label: "Pipeline", ShortGlyph: "PI", Family: "agent"}
	case ParadigmRewoo:
		return ParadigmMeta{Label: "Rewoo", ShortGlyph: "RW", Family: "agent"}
	case ParadigmGoalcon:
		return ParadigmMeta{Label: "Goalcon", ShortGlyph: "GC", Family: "agent"}
	case ParadigmEuclo:
		return ParadigmMeta{Label: "Euclo", ShortGlyph: "EU", Family: "meta"}
	default:
		return ParadigmMeta{Label: string(p), ShortGlyph: "??", Family: "unknown"}
	}
}
