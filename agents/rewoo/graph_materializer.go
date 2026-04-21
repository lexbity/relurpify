package rewoo

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/graph"
)

// ParallelGroup represents a set of steps that can execute concurrently.
type ParallelGroup struct {
	Steps []RewooStep
}

// DetectParallelGroups analyzes a plan and returns groups of steps that can run in parallel.
// Steps in the same group have no dependencies on each other (but may depend on earlier groups).
//
// Algorithm:
// 1. Build dependency DAG
// 2. Identify topological layers (depth = max dependencies on path to root)
// 3. Steps at same depth with no inter-dependency = parallel group
func DetectParallelGroups(plan *RewooPlan) []ParallelGroup {
	if plan == nil || len(plan.Steps) == 0 {
		return nil
	}

	// Build dependency map: step ID → direct dependencies
	depMap := make(map[string]map[string]bool, len(plan.Steps))
	for _, step := range plan.Steps {
		depMap[step.ID] = make(map[string]bool)
		for _, dep := range step.DependsOn {
			depMap[step.ID][dep] = true
		}
	}

	// Compute depth for each step (longest path to a root node)
	depths := computeDepths(plan.Steps, depMap)

	// Group steps by depth
	depthGroups := make(map[int][]RewooStep)
	for _, step := range plan.Steps {
		depth := depths[step.ID]
		depthGroups[depth] = append(depthGroups[depth], step)
	}

	// Convert to sorted groups
	groups := make([]ParallelGroup, 0)
	for depth := 0; depth < len(depthGroups)+1; depth++ {
		if steps, ok := depthGroups[depth]; ok && len(steps) > 0 {
			groups = append(groups, ParallelGroup{Steps: steps})
		}
	}

	return groups
}

// computeDepths computes the topological depth (longest dependency path) for each step.
func computeDepths(steps []RewooStep, depMap map[string]map[string]bool) map[string]int {
	depths := make(map[string]int)

	// Initialize: roots (no dependencies) have depth 0
	for _, step := range steps {
		if len(step.DependsOn) == 0 {
			depths[step.ID] = 0
		}
	}

	// Iteratively compute depths for dependent steps
	for {
		updated := false
		for _, step := range steps {
			if _, computed := depths[step.ID]; computed {
				continue // Already computed
			}

			// Check if all dependencies have computed depths
			maxDepth := -1
			allComputed := true
			for dep := range depMap[step.ID] {
				if depDepth, ok := depths[dep]; ok {
					if depDepth > maxDepth {
						maxDepth = depDepth
					}
				} else {
					allComputed = false
					break
				}
			}

			if allComputed {
				depths[step.ID] = maxDepth + 1
				updated = true
			}
		}

		if !updated {
			break
		}
	}

	return depths
}

// MaterializePlanGraph inserts step nodes into an existing graph based on plan dependencies.
// It uses parallel edges for steps that can execute concurrently.
//
// This function:
// 1. Detects parallel groups
// 2. Creates step nodes
// 3. Wires edges: non-parallel for sequential steps, parallel for concurrent steps
// 4. Wires all final steps to aggregate node
func MaterializePlanGraph(
	g *graph.Graph,
	plan *RewooPlan,
	registry *capability.Registry,
	permissionManager interface{}, // *authorization.PermissionManager
	options RewooOptions,
	debugf func(string, ...interface{}),
) error {
	if plan == nil || len(plan.Steps) == 0 {
		// No steps: wire plan directly to aggregate
		return g.AddEdge("rewoo_plan", "rewoo_aggregate", nil, false)
	}

	// Detect parallelizable groups
	groups := DetectParallelGroups(plan)
	if debugf != nil {
		debugf("detected %d parallel groups with %d total steps", len(groups), len(plan.Steps))
	}

	// Track which steps have been wired
	stepNodesByID := make(map[string]string, len(plan.Steps)) // step ID → node ID

	// Create nodes for each step
	for _, step := range plan.Steps {
		nodeID := fmt.Sprintf("rewoo_step_%s", step.ID)
		stepNode := NewStepNode(nodeID, step, registry, options.OnFailure)
		stepNode.OnPermissionDenied = StepOnFailureAbort
		if permissionManager != nil {
			stepNode.SetPermissionManager(permissionManager)
		}
		stepNode.Debugf = debugf

		if err := g.AddNode(stepNode); err != nil {
			return fmt.Errorf("materialize_plan: add node %s failed: %w", nodeID, err)
		}
		stepNodesByID[step.ID] = nodeID
	}

	// Wire edges based on groups
	var prevGroupSteps []RewooStep // Steps from previous group
	for _, group := range groups {
		// Connect previous group to current group
		for _, currentStep := range group.Steps {
			currentNodeID := stepNodesByID[currentStep.ID]

			if len(prevGroupSteps) == 0 {
				// First group: connect from plan_node
				if err := g.AddEdge("rewoo_plan", currentNodeID, nil, false); err != nil {
					return fmt.Errorf("materialize_plan: add plan→%s edge failed: %w", currentNodeID, err)
				}
			} else {
				// Connect from each dependency
				for _, dep := range currentStep.DependsOn {
					depNodeID := stepNodesByID[dep]
					// Use non-parallel for explicit dependencies
					if err := g.AddEdge(depNodeID, currentNodeID, nil, false); err != nil {
						return fmt.Errorf("materialize_plan: add %s→%s edge failed: %w", depNodeID, currentNodeID, err)
					}
				}

				// For steps without explicit dependency on prev group but in same depth layer,
				// add parallel edges from common ancestor
				if len(currentStep.DependsOn) == 0 && len(prevGroupSteps) > 0 {
					// Find common ancestor (first step in prev group if no explicit deps)
					ancestorNodeID := stepNodesByID[prevGroupSteps[0].ID]
					if err := g.AddEdge(ancestorNodeID, currentNodeID, nil, true); err != nil {
						return fmt.Errorf("materialize_plan: add %s→%s parallel edge failed: %w", ancestorNodeID, currentNodeID, err)
					}
				}
			}
		}

		prevGroupSteps = group.Steps
	}

	// Wire final steps to aggregate node
	// These are steps with no dependents (no other step depends on them)
	dependents := make(map[string]bool)
	for _, step := range plan.Steps {
		for _, dep := range step.DependsOn {
			dependents[dep] = true
		}
	}

	for _, step := range plan.Steps {
		if !dependents[step.ID] {
			// This step has no dependents, wire to aggregate
			nodeID := stepNodesByID[step.ID]
			if err := g.AddEdge(nodeID, "rewoo_aggregate", nil, false); err != nil {
				return fmt.Errorf("materialize_plan: add %s→aggregate edge failed: %w", nodeID, err)
			}
		}
	}

	if debugf != nil {
		debugf("materialized %d step nodes with parallel edges", len(plan.Steps))
	}

	return nil
}
