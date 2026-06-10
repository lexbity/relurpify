package thoughtrecipe

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/governance/risk"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

// TriggerPolicyRequirements captures the requested effects declared by a thoughtrecipe trigger.
type TriggerPolicyRequirements struct {
	ReadWorkspace  bool
	WriteWorkspace bool
	AskUser        bool
}

// TriggerAssociationMetadata captures trigger-local search tags.
type TriggerAssociationMetadata struct {
	Families       []string
	Keywords       []string
	HandoffTargets []string
	Tags           []string
}

// CapabilityInvocationPlan lowers a source-level capability invocation into a
// runtime-oriented call shape without resolving execution.
type CapabilityInvocationPlan struct {
	CapabilityID string
	Target       string
	Input        string
	Arguments    map[string]any
}

// LowerCapabilityInvocation normalizes a source-level capability invocation.
func LowerCapabilityInvocation(inv *CapabilityInvocation) (*CapabilityInvocationPlan, error) {
	if inv == nil {
		return nil, fmt.Errorf("capability invocation is nil")
	}
	if strings.TrimSpace(inv.Namespace.Value) != "relurpic" {
		return nil, fmt.Errorf("%s:%d:%d: unsupported capability namespace %q", inv.GetSpan().Start.File, inv.GetSpan().Start.Line, inv.GetSpan().Start.Column, inv.Namespace.Value)
	}
	capabilityID := NormalizeCapabilityReference(inv.Capability.Value)
	if capabilityID == "" {
		return nil, fmt.Errorf("%s:%d:%d: capability name is required", inv.GetSpan().Start.File, inv.GetSpan().Start.Line, inv.GetSpan().Start.Column)
	}

	plan := &CapabilityInvocationPlan{
		CapabilityID: capabilityID,
		Arguments:    make(map[string]any, 2),
	}

	if inv.Target != nil {
		ref, err := lowerInvocationReference(inv.Target, "on")
		if err != nil {
			return nil, err
		}
		plan.Target = ref
		plan.Arguments["target"] = ref
	}
	if inv.Input != nil {
		ref, err := lowerInvocationReference(inv.Input, "with")
		if err != nil {
			return nil, err
		}
		plan.Input = ref
		plan.Arguments["input"] = ref
	}

	if len(plan.Arguments) == 0 {
		plan.Arguments = nil
	}
	return plan, nil
}

// NormalizeCapabilityReference turns a source capability name into the canonical relurpic ID.
func NormalizeCapabilityReference(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if strings.Contains(name, ":") {
		return name
	}
	return "euclo:cap." + name
}

// TriggerPolicyFromDecl converts a trigger declaration into required effect flags.
func TriggerPolicyFromDecl(decl *TriggerDecl) (TriggerPolicyRequirements, error) {
	if decl == nil {
		return TriggerPolicyRequirements{}, nil
	}
	req := TriggerPolicyRequirements{}
	for _, line := range decl.Lines {
		effect := strings.ToLower(strings.TrimSpace(line.Effect.Value))
		resource := strings.ToLower(strings.TrimSpace(valueExprRaw(line.Resource)))
		switch effect {
		case "read":
			if resource != "workspace" {
				return TriggerPolicyRequirements{}, fmt.Errorf("%s:%d:%d: may read requires workspace", line.GetSpan().Start.File, line.GetSpan().Start.Line, line.GetSpan().Start.Column)
			}
			req.ReadWorkspace = true
		case "write":
			if resource != "workspace" {
				return TriggerPolicyRequirements{}, fmt.Errorf("%s:%d:%d: may write requires workspace", line.GetSpan().Start.File, line.GetSpan().Start.Line, line.GetSpan().Start.Column)
			}
			req.WriteWorkspace = true
		case "ask":
			if resource != "user" {
				return TriggerPolicyRequirements{}, fmt.Errorf("%s:%d:%d: may ask requires user", line.GetSpan().Start.File, line.GetSpan().Start.Line, line.GetSpan().Start.Column)
			}
			req.AskUser = true
		default:
			return TriggerPolicyRequirements{}, fmt.Errorf("%s:%d:%d: unsupported trigger effect %q", line.GetSpan().Start.File, line.GetSpan().Start.Line, line.GetSpan().Start.Column, line.Effect.Value)
		}
	}
	return req, nil
}

// TriggerAssociationsFromDecl converts trigger metadata lines into canonical tags.
func TriggerAssociationsFromDecl(decl *TriggerDecl) (TriggerAssociationMetadata, error) {
	if decl == nil {
		return TriggerAssociationMetadata{}, nil
	}

	meta := TriggerAssociationMetadata{}
	seenFamilies := make(map[string]struct{})
	seenKeywords := make(map[string]struct{})
	seenHandoffs := make(map[string]struct{})
	seenTags := make(map[string]struct{})

	for _, assoc := range decl.Associations {
		kind := strings.ToLower(strings.TrimSpace(assoc.Name.Value))
		if !IsSupportedTriggerAssociation(kind) {
			return TriggerAssociationMetadata{}, fmt.Errorf("%s:%d:%d: unsupported trigger association %q", assoc.GetSpan().Start.File, assoc.GetSpan().Start.Line, assoc.GetSpan().Start.Column, assoc.Name.Value)
		}
		if assoc.Values == nil {
			return TriggerAssociationMetadata{}, fmt.Errorf("%s:%d:%d: trigger %s requires a list", assoc.GetSpan().Start.File, assoc.GetSpan().Start.Line, assoc.GetSpan().Start.Column, kind)
		}
		for _, value := range assoc.Values.Entries {
			tag := normalizeTriggerTag(valueExprRaw(value))
			if tag == "" {
				return TriggerAssociationMetadata{}, fmt.Errorf("%s:%d:%d: trigger %s contains an empty tag", assoc.GetSpan().Start.File, assoc.GetSpan().Start.Line, assoc.GetSpan().Start.Column, kind)
			}
			switch kind {
			case TriggerAssociationFamily:
				if _, ok := seenFamilies[tag]; ok {
					continue
				}
				seenFamilies[tag] = struct{}{}
				meta.Families = append(meta.Families, tag)
			case TriggerAssociationKeyword:
				if _, ok := seenKeywords[tag]; ok {
					continue
				}
				seenKeywords[tag] = struct{}{}
				meta.Keywords = append(meta.Keywords, tag)
			case TriggerAssociationHandoff:
				if _, ok := seenHandoffs[tag]; ok {
					continue
				}
				seenHandoffs[tag] = struct{}{}
				meta.HandoffTargets = append(meta.HandoffTargets, tag)
			}
			if _, ok := seenTags[tag]; !ok {
				seenTags[tag] = struct{}{}
				meta.Tags = append(meta.Tags, tag)
			}
		}
	}

	return meta, nil
}

// TriggerRouteKindFromDecl returns the declared route kind for a trigger.
func TriggerRouteKindFromDecl(decl *TriggerDecl) surface.TriggerRouteKind {
	if decl == nil {
		return surface.TriggerRouteKindUnknown
	}
	kind := surface.TriggerRouteKind(strings.ToLower(strings.TrimSpace(string(decl.RouteKind))))
	if kind == "" {
		kind = surface.TriggerRouteKind(strings.ToLower(strings.TrimSpace(decl.Policy.Value)))
	}
	switch kind {
	case surface.TriggerRouteKindCapability, surface.TriggerRouteKindIntent:
		return kind
	default:
		return surface.TriggerRouteKindUnknown
	}
}

func normalizeTriggerTag(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = unquoteString(value)
	return strings.ToLower(strings.TrimSpace(value))
}

// CapabilityRequirementsFromDescriptor derives the requested effect envelope for a capability.
func CapabilityRequirementsFromDescriptor(desc descriptor.CapabilityDescriptor) TriggerPolicyRequirements {
	req := TriggerPolicyRequirements{ReadWorkspace: true}
	if capabilityRequiresWrite(desc) {
		req.WriteWorkspace = true
	}
	return req
}

// CapabilityRequirementsSatisfied checks whether the requested trigger policy covers the capability.
func CapabilityRequirementsSatisfied(trigger TriggerPolicyRequirements, capability TriggerPolicyRequirements) bool {
	if capability.ReadWorkspace && !trigger.ReadWorkspace {
		return false
	}
	if capability.WriteWorkspace && !trigger.WriteWorkspace {
		return false
	}
	if capability.AskUser && !trigger.AskUser {
		return false
	}
	return true
}

func capabilityRequiresWrite(desc descriptor.CapabilityDescriptor) bool {
	for _, effect := range desc.EffectClasses {
		switch effect {
		case classification.EffectClassFilesystemMutation,
			classification.EffectClassProcessSpawn,
			classification.EffectClassNetworkEgress,
			classification.EffectClassCredentialUse,
			classification.EffectClassExternalState,
			classification.EffectClassSessionCreation:
			return true
		}
	}
	for _, rc := range risk.Classify(desc.EffectClasses, desc.Source.Scope) {
		switch rc {
		case risk.RiskClassDestructive,
			risk.RiskClassExecute,
			risk.RiskClassNetwork,
			risk.RiskClassCredentialed,
			risk.RiskClassExfiltration,
			risk.RiskClassSessioned:
			return true
		}
	}
	category := strings.ToLower(strings.TrimSpace(desc.Category))
	for _, tag := range desc.Tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		switch tag {
		case "write", "mutating", "destructive", "refactor", "shell", "process":
			return true
		}
	}
	if strings.Contains(category, "refactor") || strings.Contains(category, "patch") || strings.Contains(category, "testing") {
		return true
	}
	return false
}

func lowerInvocationReference(expr ValueExpr, clause string) (string, error) {
	path, ok := valueExprPath(expr)
	if !ok {
		return "", fmt.Errorf("%s:%d:%d: %s operand must be a reference", expr.GetSpan().Start.File, expr.GetSpan().Start.Line, expr.GetSpan().Start.Column, clause)
	}
	if len(path.Parts) < 2 {
		return "", fmt.Errorf("%s:%d:%d: %s operand must be a namespaced reference", expr.GetSpan().Start.File, expr.GetSpan().Start.Line, expr.GetSpan().Start.Column, clause)
	}
	namespace := strings.ToLower(strings.TrimSpace(path.Parts[0].Value))
	switch namespace {
	case "input", "state", "scratch", "user", "output":
		return valueExprRaw(expr), nil
	default:
		return "", fmt.Errorf("%s:%d:%d: %s operand must use input, state, scratch, user, or output", expr.GetSpan().Start.File, expr.GetSpan().Start.Line, expr.GetSpan().Start.Column, clause)
	}
}
