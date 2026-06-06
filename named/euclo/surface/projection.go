package surface

import (
	"sort"
	"time"
)

// RecipeProjection is a UX-agnostic flattened view of a ThoughtRecipe suitable
// for frontend rendering. It carries recipe metadata, the step graph (with group
// topology), HITL gates, and route selection info.
type RecipeProjection struct {
	RecipeID      string          `json:"recipe_id"`
	Name          string          `json:"name"`
	RouteKind     string          `json:"route_kind"`
	SelectedRoute string          `json:"selected_route,omitempty"`
	FamilyID      string          `json:"family_id,omitempty"`
	Steps         []ProjectedStep `json:"steps"`
	Groups        []ProjectedGroup `json:"groups,omitempty"`
	HITLGates     []string        `json:"hitl_gates,omitempty"`
	GeneratedAt   time.Time       `json:"generated_at"`
}

// ProjectedStep is a single step in the recipe projection with runtime context.
type ProjectedStep struct {
	StepID       string   `json:"step_id"`
	Type         string   `json:"type"`
	Paradigm     string   `json:"paradigm"`
	Goal         string   `json:"goal,omitempty"`
	CapabilityID string   `json:"capability_id,omitempty"`
	ToolScopes   []string `json:"tool_scopes,omitempty"`
	HITL         string   `json:"hitl,omitempty"`
	DependsOn    []string `json:"depends_on,omitempty"`
	GroupID      string   `json:"group_id,omitempty"`
	Optional     bool     `json:"optional,omitempty"`
}

// ProjectedGroup describes a parallel, conditional, pipeline, or route group
// in the recipe topology.
type ProjectedGroup struct {
	GroupID       string   `json:"group_id"`
	Kind          string   `json:"kind"`
	MemberStepIDs []string `json:"member_step_ids"`
	Condition     string   `json:"condition,omitempty"`
	Merge         string   `json:"merge,omitempty"`
}

// BuildRecipeProjection constructs a RecipeProjection from a ThoughtRecipe and
// its structural components. The steps are processed in the order provided.
func BuildRecipeProjection(r *ThoughtRecipe, selectedRoute string, steps []ThoughtRecipeStep, parallelGroups []ParallelGroup, conditionalGroups []ConditionalGroup) RecipeProjection {
	proj := RecipeProjection{
		RecipeID:      r.ID,
		Name:          r.EffectiveName(),
		RouteKind:     string(r.RouteKind),
		SelectedRoute: selectedRoute,
		GeneratedAt:   time.Now().UTC(),
	}

	// Populate family from metadata.
	if len(r.Metadata.Families) > 0 {
		proj.FamilyID = r.Metadata.Families[0]
	}

	// Track group membership for step assignment.
	stepGroup := make(map[string]string, len(steps))
	var projectedGroups []ProjectedGroup

	// Build parallel group projections.
	for _, pg := range parallelGroups {
		memberIDs := make([]string, 0, len(pg.Steps))
		for _, s := range pg.Steps {
			memberIDs = append(memberIDs, s.ID)
			stepGroup[s.ID] = pg.ID
		}
		projectedGroups = append(projectedGroups, ProjectedGroup{
			GroupID:       pg.ID,
			Kind:          "parallel",
			MemberStepIDs: memberIDs,
			Merge:         string(pg.Merge),
		})
	}

	// Build conditional group projections.
	for _, cg := range conditionalGroups {
		memberIDs := make([]string, 0, len(cg.If)+len(cg.Else))
		for _, s := range cg.If {
			memberIDs = append(memberIDs, s.ID)
			stepGroup[s.ID] = cg.ID
		}
		for _, s := range cg.Else {
			memberIDs = append(memberIDs, s.ID)
			stepGroup[s.ID] = cg.ID
		}
		projectedGroups = append(projectedGroups, ProjectedGroup{
			GroupID:       cg.ID,
			Kind:          "conditional",
			MemberStepIDs: memberIDs,
			Condition:     cg.Condition,
		})
	}

	proj.Groups = projectedGroups

	// Collect step-level projections and HITL gates.
	hitlGates := make([]string, 0)
	projectedSteps := make([]ProjectedStep, 0, len(steps))
	seenGate := make(map[string]bool)

	for _, s := range steps {
		ps := ProjectedStep{
			StepID:       s.ID,
			Type:         s.Type,
			Paradigm:     s.Parent.Paradigm,
			Goal:         pickGoal(s),
			CapabilityID: s.CapabilityID,
			HITL:         s.HITL,
			DependsOn:    append([]string(nil), s.Dependencies...),
			GroupID:      stepGroup[s.ID],
			Optional:     false,
		}

		if isOptionalStep(s.ID, conditionalGroups) {
			ps.Optional = true
		}

		ps.ToolScopes = extractToolScopes(s)

		projectedSteps = append(projectedSteps, ps)

		if s.HITL != "" && !seenGate[s.ID] {
			hitlGates = append(hitlGates, s.ID)
			seenGate[s.ID] = true
		}
	}

	sort.Strings(hitlGates)

	proj.Steps = projectedSteps
	proj.HITLGates = hitlGates

	return proj
}

func pickGoal(s ThoughtRecipeStep) string {
	if s.Description != "" {
		return s.Description
	}
	if s.Prompt != "" {
		return s.Prompt
	}
	return ""
}

func extractToolScopes(s ThoughtRecipeStep) []string {
	return nil
}

func isOptionalStep(stepID string, groups []ConditionalGroup) bool {
	for _, g := range groups {
		for _, s := range g.Else {
			if s.ID == stepID {
				return true
			}
		}
	}
	return false
}

func stringSliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
