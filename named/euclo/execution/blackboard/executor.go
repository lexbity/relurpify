package blackboard

import (
	"context"
	"fmt"
	"sort"

	agentblackboard "codeburg.org/lexbit/relurpify/agents/blackboard"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/runtime/statebus"
)

type Result struct {
	Board       *agentblackboard.Blackboard
	Artifacts   []euclotypes.Artifact
	Cycles      int
	Termination string
	LastSource  string
}

func Execute(
	ctx context.Context,
	env euclotypes.ExecutionEnvelope,
	semctx euclotypes.ExecutorSemanticContext,
	sources []agentblackboard.KnowledgeSource,
	maxCycles int,
	terminationPredicate func(*agentblackboard.Blackboard) bool,
) (*Result, error) {
	if len(sources) == 0 {
		return nil, fmt.Errorf("blackboard execution requires at least one knowledge source")
	}
	if maxCycles <= 0 {
		maxCycles = 20
	}
	board := agentblackboard.NewBlackboard(taskInstruction(env.Task))

	// Seed from semantic context before other seeding and KS cycle begins
	if !semctx.AgentSemanticContext.IsEmpty() {
		agentblackboard.SeedBlackboardFromSemanticContext(board, semctx.AgentSemanticContext)
	}

	bridge := NewArtifactBridge(board)
	if err := bridge.SeedFromArtifacts(collectArtifacts(env.State)); err != nil {
		return nil, err
	}
	seedFromState(board, env.State)

	var lastSource, termination string
	cycles := 0
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
			termination = "no_eligible_sources"
			break
		}
		selected := eligible[0]
		if err := selected.Execute(ctx, board, env.Registry, env.Environment.Model, semctx.AgentSemanticContext); err != nil {
			publishState(env.State, board, cycles+1, maxCycles, "source_failed", selected.Name())
			return nil, fmt.Errorf("blackboard knowledge source %q failed: %w", selected.Name(), err)
		}
		lastSource = selected.Name()
		publishState(env.State, board, cycles+1, maxCycles, "running", lastSource)
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
		publishState(env.State, board, cycles, maxCycles, termination, lastSource)
	}
	return &Result{
		Board:       board.Clone(),
		Artifacts:   artifacts,
		Cycles:      cycles,
		Termination: termination,
		LastSource:  lastSource,
	}, nil
}

func seedFromState(board *agentblackboard.Blackboard, state *core.Context) {
	if board == nil || state == nil {
		return
	}
	raw, ok := statebus.GetAny(state, "euclo.blackboard_seed_facts")
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

func collectArtifacts(state *core.Context) euclotypes.ArtifactState {
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

func eligibleKnowledgeSources(sources []agentblackboard.KnowledgeSource, board *agentblackboard.Blackboard) []agentblackboard.KnowledgeSource {
	eligible := make([]agentblackboard.KnowledgeSource, 0, len(sources))
	for _, source := range sources {
		if source.CanActivate(board) {
			eligible = append(eligible, source)
		}
	}
	sort.Slice(eligible, func(i, j int) bool {
		left := agentblackboard.ResolveKnowledgeSource(eligible[i])
		right := agentblackboard.ResolveKnowledgeSource(eligible[j])
		if left.Spec.Priority == right.Spec.Priority {
			return left.Spec.Name < right.Spec.Name
		}
		return left.Spec.Priority > right.Spec.Priority
	})
	return eligible
}

func publishState(state *core.Context, board *agentblackboard.Blackboard, cycle, maxCycles int, termination, lastSource string) {
	if state == nil || board == nil {
		return
	}
	controller := agentblackboard.ControllerState{
		Cycle:       cycle,
		MaxCycles:   max(maxCycles, 1),
		Termination: termination,
		LastSource:  lastSource,
	}
	agentblackboard.PublishToContext(state, board, controller)
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func taskInstruction(task *core.Task) string {
	if task == nil || task.Instruction == "" {
		return "the requested change"
	}
	return task.Instruction
}
