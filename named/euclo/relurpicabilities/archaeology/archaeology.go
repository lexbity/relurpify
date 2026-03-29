package archaeology

import euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"

const (
	Explore              = euclorelurpic.CapabilityArchaeologyExplore
	CompilePlan          = euclorelurpic.CapabilityArchaeologyCompilePlan
	ImplementPlan        = euclorelurpic.CapabilityArchaeologyImplement
	PatternSurface       = euclorelurpic.CapabilityArchaeologyPatternSurface
	ProspectiveAssess    = euclorelurpic.CapabilityArchaeologyProspectiveAssess
	ConvergenceGuard     = euclorelurpic.CapabilityArchaeologyConvergenceGuard
	CoherenceAssess      = euclorelurpic.CapabilityArchaeologyCoherenceAssess
	ScopeExpansionAssess = euclorelurpic.CapabilityArchaeologyScopeExpand
)

type Descriptor = euclorelurpic.Descriptor

func Descriptors() []Descriptor {
	reg := euclorelurpic.DefaultRegistry()
	ids := reg.IDsForMode("planning")
	out := make([]Descriptor, 0, len(ids))
	for _, id := range ids {
		if desc, ok := reg.Lookup(id); ok {
			out = append(out, desc)
		}
	}
	return out
}
