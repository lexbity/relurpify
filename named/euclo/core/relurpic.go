package core

import euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"

type MutabilityContract = euclorelurpic.MutabilityContract
type RelurpicDescriptor = euclorelurpic.Descriptor
type RelurpicRegistry = euclorelurpic.Registry

const (
	MutabilityNonMutating        = euclorelurpic.MutabilityNonMutating
	MutabilityInspectFirst       = euclorelurpic.MutabilityInspectFirst
	MutabilityPolicyConstrained  = euclorelurpic.MutabilityPolicyConstrained
	MutabilityPlanBoundExecution = euclorelurpic.MutabilityPlanBoundExecution
)

var NewRelurpicRegistry = euclorelurpic.NewRegistry
var DefaultRelurpicRegistry = euclorelurpic.DefaultRegistry
