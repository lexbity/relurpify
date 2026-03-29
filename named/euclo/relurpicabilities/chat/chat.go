package chat

import euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"

const (
	Ask                  = euclorelurpic.CapabilityChatAsk
	Implement            = euclorelurpic.CapabilityChatImplement
	Inspect              = euclorelurpic.CapabilityChatInspect
	DirectEditExecution  = euclorelurpic.CapabilityChatDirectEditExecution
	LocalReview          = euclorelurpic.CapabilityChatLocalReview
	TargetedVerification = euclorelurpic.CapabilityChatTargetedVerification
)

type Descriptor = euclorelurpic.Descriptor

func Descriptors() []Descriptor {
	reg := euclorelurpic.DefaultRegistry()
	ids := reg.IDsForMode("chat")
	out := make([]Descriptor, 0, len(ids))
	for _, id := range ids {
		if desc, ok := reg.Lookup(id); ok {
			out = append(out, desc)
		}
	}
	return out
}
