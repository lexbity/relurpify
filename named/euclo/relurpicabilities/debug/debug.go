package debug

import euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"

const (
	InvestigateRepair  = euclorelurpic.CapabilityDebugInvestigateRepair
	SimpleRepair       = euclorelurpic.CapabilityDebugRepairSimple
	RootCause          = euclorelurpic.CapabilityDebugRootCause
	HypothesisRefine   = euclorelurpic.CapabilityDebugHypothesisRefine
	Localization       = euclorelurpic.CapabilityDebugLocalization
	FlawSurface        = euclorelurpic.CapabilityDebugFlawSurface
	VerificationRepair = euclorelurpic.CapabilityDebugVerificationRepair
)

type Descriptor = euclorelurpic.Descriptor

func Descriptors() []Descriptor {
	reg := euclorelurpic.DefaultRegistry()
	ids := reg.IDsForMode("debug")
	out := make([]Descriptor, 0, len(ids))
	for _, id := range ids {
		if desc, ok := reg.Lookup(id); ok {
			out = append(out, desc)
		}
	}
	return out
}
