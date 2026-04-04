package eternal

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/lexcodex/relurpify/ayenitd"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

type Agent struct {
	Model  core.LanguageModel
	Config *core.Config

	MaxTokensPerCycle int
	ResetDuration     time.Duration
	Infinite          bool
	MaxCycles         int
	SleepPerCycle     time.Duration
}

func New(env ayenitd.WorkspaceEnvironment) *Agent {
	agent := &Agent{}
	_ = agent.InitializeEnvironment(env)
	return agent
}

func (a *Agent) InitializeEnvironment(env ayenitd.WorkspaceEnvironment) error {
	a.Model = env.Model
	a.Config = env.Config
	return a.Initialize(env.Config)
}

func (a *Agent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.MaxTokensPerCycle <= 0 {
		a.MaxTokensPerCycle = 512
	}
	if a.ResetDuration <= 0 {
		a.ResetDuration = 1 * time.Hour
	}
	if a.MaxCycles <= 0 {
		a.MaxCycles = 1
	}
	a.Infinite = false
	if a.SleepPerCycle < 0 {
		a.SleepPerCycle = 0
	}
	return nil
}

func (a *Agent) Capabilities() []core.Capability { return nil }

func (a *Agent) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("eternal_done")
	_ = g.AddNode(done)
	_ = g.SetStart(done.ID())
	return g, nil
}

func (a *Agent) Execute(ctx context.Context, task *core.Task, _ *core.Context) (*core.Result, error) {
	systemPrompt := `Assistant is in a CLI mood today.`
	currentPrompt := task.Instruction
	if currentPrompt == "" {
		currentPrompt = "initiate sequence"
	}
	infinite := a.Infinite
	maxCycles := a.MaxCycles
	sleepPerCycle := a.SleepPerCycle
	if task != nil && task.Context != nil {
		if raw, ok := task.Context["eternal.infinite"]; ok {
			if v, ok := raw.(bool); ok {
				infinite = v
			}
		}
		if raw, ok := task.Context["eternal.max_cycles"]; ok {
			switch v := raw.(type) {
			case int:
				maxCycles = v
			case int64:
				maxCycles = int(v)
			case float64:
				maxCycles = int(v)
			case string:
				if parsed, err := strconv.Atoi(v); err == nil {
					maxCycles = parsed
				}
			}
		}
	}
	_ = systemPrompt
	cycles := 0
	for {
		cycles++
		if !infinite && cycles >= maxCycles {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(sleepPerCycle):
			currentPrompt = fmt.Sprintf("%s -> cycle %d", currentPrompt, cycles)
		}
	}
	return &core.Result{Success: true, Data: map[string]any{"text": currentPrompt, "cycles": cycles}}, nil
}
