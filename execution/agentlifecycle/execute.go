package agentlifecycle

import (
	"context"
	"errors"
	"fmt"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/governance/authorization"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
)

// ExecuteConfig holds the dependencies for ExecuteAgent.
type ExecuteConfig struct {
	AgentID         string
	AgentExecutor   execution.AgentExecutor
	Enforcer        authorization.Enforcer
}

// ExecuteAgent runs an agent under governance authorization.
// It sets the Principal context key at task entry, calls Enforcer.Check
// before execution, and audits the result.
func ExecuteAgent(ctx context.Context, cfg ExecuteConfig, task *execution.Task, state *contextdata.Envelope) (*execution.Result, error) {
	if cfg.AgentExecutor == nil {
		return nil, errors.New("agent executor missing")
	}
	if cfg.Enforcer == nil {
		return nil, errors.New("enforcer missing")
	}
	if task == nil {
		return nil, errors.New("task missing")
	}

	// Set governance Principal at task entry.
	principal := governanceports.Principal{
		AgentID: cfg.AgentID,
	}
	ctx = governanceports.ContextWithPrincipal(ctx, principal)

	// Build AccessRequest for the execution action.
	req := governanceports.AccessRequest{
		Principal: principal,
		Action:    governanceports.ActionToolInvoke,
	}
	decision := cfg.Enforcer.Check(ctx, req)
	if !decision.Allow {
		return nil, fmt.Errorf("execution denied: %s", decision.Reason)
	}

	// Initialize and execute the agent.
	if err := cfg.AgentExecutor.Initialize(&execution.Config{Name: cfg.AgentID, NativeToolCalling: true}); err != nil {
		return nil, fmt.Errorf("agent initialize: %w", err)
	}
	return cfg.AgentExecutor.Execute(ctx, task, state)
}
