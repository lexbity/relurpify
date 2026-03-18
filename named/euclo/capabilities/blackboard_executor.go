package capabilities

import (
	"context"
	"fmt"
	"sort"

	"github.com/lexcodex/relurpify/agents/blackboard"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

type BlackboardResult struct {
	Board       *blackboard.Blackboard
	Artifacts   []euclotypes.Artifact
	Cycles      int
	Termination string
	LastSource  string
}

func ExecuteBlackboard(
	ctx context.Context,
	env euclotypes.ExecutionEnvelope,
	sources []blackboard.KnowledgeSource,
	maxCycles int,
	terminationPredicate func(*blackboard.Blackboard) bool,
) (*BlackboardResult, error) {
	if len(sources) == 0 {
		return nil, fmt.Errorf("blackboard execution requires at least one knowledge source")
	}
	if maxCycles <= 0 {
		maxCycles = 20
	}

	board := blackboard.NewBlackboard(capTaskInstruction(env.Task))
	bridge := NewBlackboardArtifactBridge(board)
	if err := bridge.SeedFromArtifacts(collectEnvelopeArtifacts(env.State)); err != nil {
		return nil, err
	}
	seedBlackboardFromState(board, env.State)

	var (
		lastSource  string
		termination string
		cycles      int
	)

	for cycles = 0; cycles < maxCycles; cycles++ {
		if terminationPredicate != nil && terminationPredicate(board) {
			termination = "predicate_satisfied"
			break
		}
		if board.IsGoalSatisfied() {
			termination = "goal_satisfied"
			break
		}

		eligible := eligibleKnowledgeSources(sources, board)
		if len(eligible) == 0 {
			if terminationPredicate != nil && terminationPredicate(board) {
				termination = "predicate_satisfied"
				break
			}
			termination = "no_eligible_sources"
			break
		}

		selected := eligible[0]
		if err := selected.Execute(ctx, board, env.Registry, env.Environment.Model); err != nil {
			publishBlackboardState(env.State, board, cycles+1, maxCycles, "source_failed", selected.Name())
			return nil, fmt.Errorf("blackboard knowledge source %q failed: %w", selected.Name(), err)
		}
		lastSource = selected.Name()
		publishBlackboardState(env.State, board, cycles+1, maxCycles, "running", lastSource)
	}

	if termination == "" {
		if terminationPredicate != nil && terminationPredicate(board) {
			termination = "predicate_satisfied"
		} else if board.IsGoalSatisfied() {
			termination = "goal_satisfied"
		} else {
			termination = "cycle_limit"
		}
	}

	artifacts := bridge.HarvestToArtifacts()
	if env.State != nil {
		euclotypes.RestoreStateFromArtifacts(env.State, artifacts)
		publishBlackboardState(env.State, board, cycles, maxCycles, termination, lastSource)
	}

	return &BlackboardResult{
		Board:       board.Clone(),
		Artifacts:   artifacts,
		Cycles:      cycles,
		Termination: termination,
		LastSource:  lastSource,
	}, nil
}

func seedBlackboardFromState(board *blackboard.Blackboard, state *core.Context) {
	if board == nil || state == nil {
		return
	}
	raw, ok := state.Get("euclo.blackboard_seed_facts")
	if !ok || raw == nil {
		return
	}
	switch typed := raw.(type) {
	case map[string]any:
		for key, value := range typed {
			setBoardEntry(board, key, value, "euclo:blackboard_seed")
		}
	case map[string]string:
		for key, value := range typed {
			setBoardEntry(board, key, value, "euclo:blackboard_seed")
		}
	}
}

func collectEnvelopeArtifacts(state *core.Context) euclotypes.ArtifactState {
	var artifacts []euclotypes.Artifact
	seen := map[string]struct{}{}

	for _, artifact := range euclotypes.ArtifactStateFromContext(state).All() {
		key := artifact.ID + "|" + string(artifact.Kind)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		artifacts = append(artifacts, artifact)
	}
	for _, artifact := range euclotypes.CollectArtifactsFromState(state) {
		key := artifact.ID + "|" + string(artifact.Kind)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		artifacts = append(artifacts, artifact)
	}

	return euclotypes.NewArtifactState(artifacts)
}

func eligibleKnowledgeSources(sources []blackboard.KnowledgeSource, board *blackboard.Blackboard) []blackboard.KnowledgeSource {
	eligible := make([]blackboard.KnowledgeSource, 0, len(sources))
	for _, source := range sources {
		if source.CanActivate(board) {
			eligible = append(eligible, source)
		}
	}
	sort.Slice(eligible, func(i, j int) bool {
		left := blackboard.ResolveKnowledgeSource(eligible[i])
		right := blackboard.ResolveKnowledgeSource(eligible[j])
		if left.Spec.Priority == right.Spec.Priority {
			return left.Spec.Name < right.Spec.Name
		}
		return left.Spec.Priority > right.Spec.Priority
	})
	return eligible
}

func publishBlackboardState(state *core.Context, board *blackboard.Blackboard, cycle, maxCycles int, termination, lastSource string) {
	if state == nil || board == nil {
		return
	}
	controller := blackboard.ControllerState{
		Cycle:       cycle,
		MaxCycles:   max(maxCycles, 1),
		Termination: termination,
		LastSource:  lastSource,
	}
	blackboard.PublishToContext(state, board, controller)
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
