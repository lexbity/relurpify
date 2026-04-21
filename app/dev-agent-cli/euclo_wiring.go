package main

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/agents/relurpic"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	archaeotensions "codeburg.org/lexbit/relurpify/archaeo/tensions"
	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/guidance"
	frameworkskills "codeburg.org/lexbit/relurpify/framework/skills"
	"codeburg.org/lexbit/relurpify/named/euclo"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

var buildAndWireEucloAgentFn = func(ws *ayenitd.Workspace, learningBroker *archaeolearning.Broker) graph.WorkflowExecutor {
	return buildAndWireEucloAgent(ws, learningBroker)
}

func buildAndWireEucloAgent(ws *ayenitd.Workspace, learningBroker *archaeolearning.Broker) graph.WorkflowExecutor {
	if ws == nil {
		return nil
	}
	agent := euclo.New(ws.Environment)
	if agent == nil {
		return nil
	}
	env := ws.Environment
	if agent.GraphDB == nil {
		agent.GraphDB = graphDBFromEnv(env)
	}
	if agent.RetrievalDB == nil {
		agent.RetrievalDB = retrievalDBFromEnv(env)
	}
	if agent.PlanStore == nil {
		agent.PlanStore = env.PlanStore
	}
	if agent.WorkflowStore == nil {
		agent.WorkflowStore = env.WorkflowStore
	}
	if agent.GuidanceBroker == nil {
		agent.GuidanceBroker = env.GuidanceBroker
	}
	if agent.LearningBroker == nil && learningBroker != nil {
		agent.LearningBroker = learningBroker
	}
	if agent.DeferralPolicy.MaxBlastRadiusForDefer == 0 && len(agent.DeferralPolicy.DeferrableKinds) == 0 {
		agent.DeferralPolicy = guidance.DefaultDeferralPolicy()
	}
	return agent
}

func graphDBFromEnv(env ayenitd.WorkspaceEnvironment) *graphdb.Engine {
	if env.IndexManager == nil {
		return nil
	}
	return env.IndexManager.GraphDB
}

func retrievalDBFromEnv(env ayenitd.WorkspaceEnvironment) *sql.DB {
	if env.RetrievalDB != nil {
		return env.RetrievalDB
	}
	if store, ok := env.WorkflowStore.(interface{ DB() *sql.DB }); ok {
		return store.DB()
	}
	return nil
}

func isEucloAgent(agentName string, spec *core.AgentRuntimeSpec) bool {
	if spec != nil && strings.EqualFold(strings.TrimSpace(spec.Implementation), "coding") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(agentName), "euclo")
}

func eucloModeRegistry() *euclotypes.ModeRegistry {
	return euclotypes.DefaultModeRegistry()
}

func eucloModeNames(reg *euclotypes.ModeRegistry) []string {
	if reg == nil {
		return nil
	}
	modes := reg.List()
	out := make([]string, 0, len(modes))
	for _, mode := range modes {
		if mode.ModeID == "" {
			continue
		}
		out = append(out, mode.ModeID)
	}
	sort.Strings(out)
	return out
}

func validateEucloMode(mode string) error {
	trimmed := strings.TrimSpace(mode)
	if trimmed == "" {
		return nil
	}
	reg := eucloModeRegistry()
	if _, ok := reg.Lookup(trimmed); ok {
		return nil
	}
	return fmt.Errorf("unknown euclo mode %q; valid modes: %s", trimmed, strings.Join(eucloModeNames(reg), ", "))
}

func eucloReadyHint(agentName string) string {
	return fmt.Sprintf(
		"Agent %s ready. Available modes: %s\nUse --mode <name> or set spec.agent.mode in the manifest.\n",
		agentName, strings.Join(eucloModeNames(eucloModeRegistry()), ", "),
	)
}

func collectEucloArtifactPaths(skillResults []frameworkskills.SkillResolution) []string {
	out := make([]string, 0, len(skillResults))
	for _, skill := range skillResults {
		if !skill.Applied || strings.TrimSpace(skill.Paths.Root) == "" {
			continue
		}
		out = append(out, skill.Paths.Root)
	}
	return out
}

func collectArtifactKindsFromState(state *core.Context) []string {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("euclo.artifacts")
	if !ok || raw == nil {
		return nil
	}
	kinds := make([]string, 0)
	appendKind := func(kind string) {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			return
		}
		kinds = append(kinds, kind)
	}
	switch typed := raw.(type) {
	case []euclotypes.Artifact:
		for _, artifact := range typed {
			appendKind(string(artifact.Kind))
		}
	case []any:
		for _, item := range typed {
			switch artifact := item.(type) {
			case euclotypes.Artifact:
				appendKind(string(artifact.Kind))
			case map[string]any:
				if kind, ok := artifact["kind"].(string); ok {
					appendKind(kind)
				}
			}
		}
	default:
		if artifacts := euclotypes.CollectArtifactsFromState(state); len(artifacts) > 0 {
			for _, artifact := range artifacts {
				appendKind(string(artifact.Kind))
			}
		}
	}
	return kinds
}

func eucloResolvedMode(state *core.Context) string {
	if state == nil {
		return ""
	}
	raw, ok := state.Get("euclo.mode_resolution")
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case eucloruntime.ModeResolution:
		return strings.TrimSpace(typed.ModeID)
	case map[string]any:
		if v, ok := typed["resolved_mode"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		if v, ok := typed["mode_id"].(string); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func eucloInteractionRecorded(state *core.Context) bool {
	if state == nil {
		return false
	}
	raw, ok := state.Get("euclo.interaction_recording")
	if !ok || raw == nil {
		return false
	}
	switch typed := raw.(type) {
	case map[string]any:
		if count, ok := typed["event_count"].(int); ok {
			return count > 0
		}
		if count, ok := typed["event_count"].(float64); ok {
			return count > 0
		}
		return true
	default:
		return true
	}
}

type executionSummary struct {
	TaskID        string   `json:"task_id"`
	Mode          string   `json:"mode"`
	ResultNode    string   `json:"result_node"`
	ElapsedMillis int64    `json:"elapsed_ms"`
	ArtifactPaths []string `json:"artifact_paths,omitempty"`
	ArtifactKinds []string `json:"artifact_kinds,omitempty"`
	Recorded      bool     `json:"recorded,omitempty"`
	Success       bool     `json:"success"`
}

func buildExecutionSummary(taskID, mode string, result *core.Result, state *core.Context, skillResults []frameworkskills.SkillResolution, elapsed time.Duration, eucloOutput bool) executionSummary {
	summary := executionSummary{
		TaskID:        taskID,
		Mode:          strings.TrimSpace(mode),
		ElapsedMillis: elapsed.Milliseconds(),
		Success:       result != nil && result.Success,
	}
	if result != nil {
		summary.ResultNode = result.NodeID
	}
	if eucloOutput {
		summary.Mode = eucloResolvedMode(state)
		if summary.Mode == "" {
			summary.Mode = strings.TrimSpace(mode)
		}
		summary.ArtifactKinds = collectArtifactKindsFromState(state)
		summary.Recorded = eucloInteractionRecorded(state)
	}
	summary.ArtifactPaths = collectEucloArtifactPaths(skillResults)
	return summary
}
