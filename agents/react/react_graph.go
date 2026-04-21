package react

import (
	"fmt"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

// BuildGraph constructs the ReAct workflow.
func (a *ReActAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	if a.Model == nil {
		return nil, fmt.Errorf("react agent missing language model")
	}
	think := &reactThinkNode{
		id:    "react_think",
		agent: a,
		task:  task,
	}
	act := &reactActNode{
		id:    "react_act",
		agent: a,
		task:  task,
	}
	observe := &reactObserveNode{
		id:    "react_observe",
		agent: a,
		task:  task,
	}
	done := graph.NewTerminalNode("react_done")
	summarize := graph.NewSummarizeContextNode("react_summarize", a.contextSummarizer())
	summarize.StateKeys = []string{"react.last_tool_result", "react.tool_observations", "react.final_output", "react.incomplete_reason"}
	summarize.Telemetry = telemetryForConfig(a.Config)
	var persist *graph.PersistenceWriterNode
	if reactUsesStructuredPersistence(a.Config) {
		if runtimeStore := runtimeMemoryStore(a.Memory); runtimeStore != nil {
			persist = graph.NewPersistenceWriterNode("react_persist", runtimeStore)
			persist.TaskID = taskID(task)
			persist.Telemetry = telemetryForConfig(a.Config)
			persist.Declarative = []graph.DeclarativePersistenceRequest{{
				StateKey:            "react.final_output",
				Scope:               string(memory.MemoryScopeProject),
				Kind:                graph.DeclarativeKindProjectKnowledge,
				Title:               taskInstructionText(task),
				SummaryField:        "summary",
				ContentField:        "result",
				ArtifactRefStateKey: "graph.summary_ref",
				Tags:                []string{"react", "task-summary"},
				Reason:              "react-completion-summary",
			}}
			persist.Artifacts = []graph.ArtifactPersistenceRequest{{
				ArtifactRefStateKey: "graph.summary_ref",
				SummaryStateKey:     "graph.summary",
				Reason:              "react-context-summary-artifact",
			}}
		}
	}
	var checkpoint *graph.CheckpointNode
	if reactUsesExplicitCheckpointNodes(a.Config) && a.CheckpointPath != "" && task != nil && task.ID != "" {
		checkpoint = graph.NewCheckpointNode("react_checkpoint", done.ID(), memory.NewCheckpointStore(filepath.Clean(a.CheckpointPath)))
		checkpoint.TaskID = task.ID
		checkpoint.Telemetry = telemetryForConfig(a.Config)
	}
	g := graph.NewGraph()
	if catalog := a.executionCapabilityCatalog(); catalog != nil && len(catalog.InspectableCapabilities()) > 0 {
		g.SetCapabilityCatalog(catalog)
	}
	if reactUsesDeclarativeRetrieval(a.Config) && a.Memory != nil {
		retrieve := graph.NewRetrieveDeclarativeMemoryNode("react_retrieve_declarative", scopedMemoryRetriever{
			store:       a.Memory,
			scope:       memory.MemoryScopeProject,
			memoryClass: core.MemoryClassDeclarative,
		})
		retrieve.Query = taskInstructionText(task)
		if err := g.AddNode(retrieve); err != nil {
			return nil, err
		}
		if err := g.SetStart(retrieve.ID()); err != nil {
			return nil, err
		}
		if err := g.AddNode(think); err != nil {
			return nil, err
		}
		if err := g.AddEdge(retrieve.ID(), think.ID(), nil, false); err != nil {
			return nil, err
		}
	} else {
		if err := g.AddNode(think); err != nil {
			return nil, err
		}
		if err := g.SetStart(think.ID()); err != nil {
			return nil, err
		}
	}
	for _, node := range []graph.Node{act, observe, summarize, done} {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	if persist != nil {
		if err := g.AddNode(persist); err != nil {
			return nil, err
		}
	}
	if checkpoint != nil {
		if err := g.AddNode(checkpoint); err != nil {
			return nil, err
		}
	}
	if err := g.AddEdge(think.ID(), act.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(act.ID(), observe.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(observe.ID(), think.ID(), func(result *core.Result, ctx *core.Context) bool {
		done, _ := ctx.Get("react.done")
		return done == false || done == nil
	}, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(observe.ID(), summarize.ID(), func(result *core.Result, ctx *core.Context) bool {
		done, _ := ctx.Get("react.done")
		return done == true
	}, false); err != nil {
		return nil, err
	}
	nextAfterSummarize := done.ID()
	if persist != nil {
		nextAfterSummarize = persist.ID()
	} else if checkpoint != nil {
		nextAfterSummarize = checkpoint.ID()
	}
	if err := g.AddEdge(summarize.ID(), nextAfterSummarize, nil, false); err != nil {
		return nil, err
	}
	if persist != nil && checkpoint != nil {
		if err := g.AddEdge(persist.ID(), checkpoint.ID(), nil, false); err != nil {
			return nil, err
		}
		if err := g.AddEdge(checkpoint.ID(), done.ID(), nil, false); err != nil {
			return nil, err
		}
	} else if persist != nil {
		if err := g.AddEdge(persist.ID(), done.ID(), nil, false); err != nil {
			return nil, err
		}
	} else if checkpoint != nil {
		if err := g.AddEdge(checkpoint.ID(), done.ID(), nil, false); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func (a *ReActAgent) contextSummarizer() core.Summarizer {
	if a != nil && a.contextPolicy != nil && a.contextPolicy.Summarizer != nil {
		return a.contextPolicy.Summarizer
	}
	return &core.SimpleSummarizer{}
}

func telemetryForConfig(cfg *core.Config) core.Telemetry {
	if cfg == nil {
		return nil
	}
	return cfg.Telemetry
}

func taskID(task *core.Task) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.ID)
}

func runtimeMemoryStore(store memory.MemoryStore) graph.RuntimePersistenceStore {
	if runtimeStore, ok := store.(memory.RuntimeMemoryStore); ok {
		return memory.AdaptRuntimeStoreForGraph(runtimeStore)
	}
	return nil
}

func reactUsesExplicitCheckpointNodes(cfg *core.Config) bool {
	if cfg == nil || cfg.UseExplicitCheckpointNodes == nil {
		return true
	}
	return *cfg.UseExplicitCheckpointNodes
}

func reactUsesDeclarativeRetrieval(cfg *core.Config) bool {
	if cfg == nil || cfg.UseDeclarativeRetrieval == nil {
		return true
	}
	return *cfg.UseDeclarativeRetrieval
}

func reactUsesStructuredPersistence(cfg *core.Config) bool {
	if cfg == nil || cfg.UseStructuredPersistence == nil {
		return true
	}
	return *cfg.UseStructuredPersistence
}
