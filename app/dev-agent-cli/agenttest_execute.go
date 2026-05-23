package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	appruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	namedfactory "codeburg.org/lexbit/relurpify/named/factory"
	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

var executePreparedRunAgentTaskFn = executePreparedRunAgentTask

func executePreparedRunAgentTask(ctx context.Context, ws *agentenv.Workspace, desc *agenttest.PreparedRunDescriptor, out io.Writer) (*core.Result, error) {
	if ws == nil {
		return nil, fmt.Errorf("workspace required")
	}
	if desc == nil {
		return nil, fmt.Errorf("descriptor required")
	}
	spec := ws.AgentSpec
	if spec == nil && ws.EffectiveContract != nil {
		spec = ws.EffectiveContract.AgentSpec
	}
	if spec == nil {
		return nil, fmt.Errorf("workspace agent spec missing")
	}
	if strings.TrimSpace(desc.Instruction) == "" {
		return nil, fmt.Errorf("instruction required")
	}
	if ws.Registration != nil && ws.Registration.Permissions != nil {
		ws.Registration.Permissions.SetDefaultPolicy(agentspec.AgentPermissionAllow)
	}
	if ws.Registration != nil && ws.Registration.HITL != nil {
		ws.Registration.HITL.AutoApprove = true
	}
	spec.Bash.Default = agentspec.AgentPermissionAllow

	if ws.Environment.Config == nil {
		ws.Environment.Config = &core.Config{}
	}
	ws.Environment.Config.Name = desc.AgentName
	ws.Environment.Config.Model = desc.ModelName
	ws.Environment.Config.MaxIterations = desc.MaxIterations
	ws.Environment.Config.NativeToolCalling = spec.NativeToolCallingEnabled()
	ws.Environment.Config.AgentSpec = spec

	providerRuntime := &appruntime.Runtime{
		Workspace:    ws,
		Tools:        ws.Environment.Registry,
		Model:        ws.Environment.Model,
		IndexManager: ws.Environment.IndexManager,
		SearchEngine: ws.Environment.SearchEngine,
		Memory:       ws.Environment.WorkingMemory,
	}
	if err := registerBuiltinProvidersFn(ctx, providerRuntime); err != nil {
		return nil, err
	}

	agent := namedfactory.InstantiateByName(ws.Environment.Config.Name, ws.Environment.Config.Workspace, ws.Environment)
	if agent == nil {
		return nil, fmt.Errorf("agent %s unavailable", ws.Environment.Config.Name)
	}

	execCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	task := &core.Task{
		ID:          fmt.Sprintf("cli-%d", time.Now().UnixNano()),
		Type:        string(core.TaskTypeCodeGeneration),
		Instruction: desc.Instruction,
		Context:     agentTestSurface.BuildStartTaskContext(agentTestSurface.ResolveStartMode("", spec), ws.Environment.Config.Workspace),
	}
	env := contextdata.NewEnvelope(task.ID, "")
	env.WorkingData["task.id"] = task.ID
	env.WorkingData["task.type"] = task.Type
	env.WorkingData["task.instruction"] = task.Instruction

	result, err := agent.Execute(execCtx, task, env)
	if err != nil {
		return nil, err
	}
	if out != nil {
		_, _ = fmt.Fprintf(out, "Agent complete (node=%s): %+v\n", result.NodeID, result.Data)
	}
	return result, nil
}
