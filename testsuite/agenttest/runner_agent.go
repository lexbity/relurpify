package agenttest

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/agents"
	appruntime "github.com/lexcodex/relurpify/app/relurpish/runtime"
	"github.com/lexcodex/relurpify/framework/ast"
	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/memory"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
)

func shouldSkipCase(req RequiresSpec, agent graph.Agent) (reason string, ok bool) {
	for _, bin := range req.Executables {
		bin = strings.TrimSpace(bin)
		if bin == "" {
			continue
		}
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Sprintf("missing executable %s", bin), true
		}
	}
	if len(req.Tools) > 0 {
		reg := extractCapabilityRegistry(agent)
		if reg == nil {
			return "agent has no capability registry", true
		}
		for _, name := range req.Tools {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, ok := reg.Get(name); !ok {
				return fmt.Sprintf("missing tool %s", name), true
			}
		}
	}
	return "", false
}

func extractCapabilityRegistry(agent graph.Agent) *capability.Registry {
	switch a := agent.(type) {
	case *agents.CodingAgent:
		return a.Tools
	case *agents.PlannerAgent:
		return a.Tools
	case *agents.ReActAgent:
		return a.Tools
	default:
		return nil
	}
}

func effectiveAgentSpecForCase(base *core.AgentRuntimeSpec, c CaseSpec) *core.AgentRuntimeSpec {
	if base == nil {
		base = &core.AgentRuntimeSpec{}
	}
	clone := *base

	// Agenttest defaults: keep writes safe without relying on filesystem-permission
	// rewrites (tools declare broad perms for authorization).
	clone.Files.Write.Default = core.AgentPermissionDeny
	clone.Files.Edit.Default = core.AgentPermissionDeny
	clone.Files.Write.RequireApproval = false
	clone.Files.Edit.RequireApproval = false

	allow := []string{
		"relurpify_cfg/test_runs/**",
		"relurpify_cfg/memory/**",
		"testsuite/agenttest_fixtures/**",
	}
	for _, f := range c.Setup.Files {
		if strings.TrimSpace(f.Path) == "" {
			continue
		}
		allow = append(allow, filepath.ToSlash(filepath.Clean(f.Path)))
		allow = append(allow, filepath.ToSlash(filepath.Clean(f.Path))+".bak")
	}
	for _, pat := range c.Expect.FilesChanged {
		if strings.TrimSpace(pat) == "" {
			continue
		}
		allow = append(allow, filepath.ToSlash(filepath.Clean(pat)))
		allow = append(allow, filepath.ToSlash(filepath.Clean(pat))+".bak")
	}
	clone.Files.Write.AllowPatterns = uniqueStrings(append(clone.Files.Write.AllowPatterns, allow...))
	clone.Files.Edit.AllowPatterns = uniqueStrings(append(clone.Files.Edit.AllowPatterns, allow...))
	return &clone
}

func buildAgent(workspace, manifestPath, agentName string, agentSpec *core.AgentRuntimeSpec, model core.LanguageModel, telemetry core.Telemetry, opts RunOptions, extraEnv []string, allowedCapabilities []core.CapabilitySelector, c CaseSpec, mem memory.MemoryStore) (graph.Agent, *core.Context, error) {
	agentManifest, err := manifest.LoadAgentManifest(manifestPath)
	if err != nil {
		return nil, nil, err
	}

	audit := core.NewInMemoryAuditLogger(512)
	hitl := fauthorization.NewHITLBroker(30 * time.Second)
	// Auto-approve all HITL requests in test runs — there is no human operator.
	hitlEvents, hitlCancel := hitl.Subscribe(32)
	go func() {
		defer hitlCancel()
		for event := range hitlEvents {
			if event.Type == fauthorization.HITLEventRequested && event.Request != nil {
				_ = hitl.Approve(fauthorization.PermissionDecision{
					RequestID:  event.Request.ID,
					Approved:   true,
					ApprovedBy: "agenttest-auto",
					Scope:      fauthorization.GrantScopeSession,
				})
			}
		}
	}()
	effectivePerms, err := manifest.ResolveEffectivePermissions(workspace, agentManifest)
	if err != nil {
		return nil, nil, err
	}
	agentManifest.Spec.Permissions = effectivePerms
	permMgr, err := fauthorization.NewPermissionManager(workspace, &agentManifest.Spec.Permissions, audit, hitl)
	if err != nil {
		return nil, nil, err
	}

	var runner fsandbox.CommandRunner
	if opts.Sandbox {
		reg, err := fauthorization.RegisterAgent(context.Background(), fauthorization.RuntimeConfig{
			ManifestPath: manifestPath,
			Sandbox:      appruntime.DefaultConfig().Sandbox,
			AuditLimit:   512,
			BaseFS:       workspace,
			HITLTimeout:  30 * time.Second,
		})
		if err != nil {
			return nil, nil, err
		}
		runner, err = fsandbox.NewSandboxCommandRunner(reg.Manifest, reg.Runtime, workspace)
		if err != nil {
			return nil, nil, err
		}
		permMgr = reg.Permissions
	} else {
		runner = fsandbox.NewLocalCommandRunner(workspace, extraEnv)
	}

	registry, indexManager, err := appruntime.BuildCapabilityRegistry(workspace, runner, appruntime.CapabilityRegistryOptions{
		AgentID:           agentManifest.Metadata.Name,
		PermissionManager: permMgr,
		AgentSpec:         agentSpec,
	})
	if err != nil {
		return nil, nil, err
	}
	applyAgentTestCapabilityDefaults(registry, allowedCapabilities)
	registry.UseTelemetry(telemetry)
	registry.UsePermissionManager(agentManifest.Metadata.Name, permMgr)
	registry.UseAgentSpec(agentManifest.Metadata.Name, agentSpec)

	if mem == nil {
		paths := config.New(workspace)
		mem, err = memory.NewHybridMemory(paths.MemoryDir())
		if err != nil {
			return nil, nil, err
		}
	}

	agent := instantiateAgentByName(
		workspace,
		agentName,
		model,
		registry,
		mem,
		indexManager,
	)
	if err := applyCaseControlFlowOverride(agent, c); err != nil {
		return nil, nil, err
	}

	maxIterations := opts.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 8
	}
	cfg := &core.Config{
		Name:              agentManifest.Metadata.Name,
		Model:             firstNonEmpty(opts.ModelOverride, agentSpec.Model.Name),
		OllamaEndpoint:    firstNonEmpty(opts.EndpointOverride, "http://localhost:11434"),
		MaxIterations:     maxIterations,
		OllamaToolCalling: agentSpec.ToolCallingEnabled(),
		AgentSpec:         agentSpec,
		DebugLLM:          opts.DebugLLM,
		DebugAgent:        opts.DebugAgent,
		Telemetry:         telemetry,
	}
	if err := agent.Initialize(cfg); err != nil {
		return nil, nil, err
	}
	if reflection, ok := agent.(*agents.ReflectionAgent); ok && reflection.Delegate != nil {
		_ = reflection.Delegate.Initialize(cfg)
	}
	return agent, core.NewContext(), nil
}

func applyCaseControlFlowOverride(agent graph.Agent, c CaseSpec) error {
	flow := strings.TrimSpace(c.Overrides.ControlFlow)
	if flow == "" {
		return nil
	}
	coding, ok := agent.(*agents.CodingAgent)
	if !ok {
		return nil
	}
	mode := agents.ModeCode
	if c.Context != nil {
		if raw, ok := c.Context["mode"]; ok {
			if parsed := strings.TrimSpace(fmt.Sprint(raw)); parsed != "" {
				mode = agents.Mode(strings.ToLower(parsed))
			}
		}
	}
	switch strings.ToLower(flow) {
	case string(agents.ControlFlowPipeline):
		return coding.OverrideControlFlow(mode, agents.ControlFlowPipeline)
	case string(agents.ControlFlowArchitect):
		return coding.OverrideControlFlow(mode, agents.ControlFlowArchitect)
	case string(agents.ControlFlowReAct):
		return coding.OverrideControlFlow(mode, agents.ControlFlowReAct)
	default:
		return fmt.Errorf("unsupported control_flow override %q", flow)
	}
}

func defaultAgenttestAllowedCapabilities() []core.CapabilitySelector {
	return []core.CapabilitySelector{
		{Name: "browser", Kind: core.CapabilityKindTool},
		{Name: "file_read", Kind: core.CapabilityKindTool},
		{Name: "file_list", Kind: core.CapabilityKindTool},
		{Name: "file_search", Kind: core.CapabilityKindTool},
		{Name: "file_create", Kind: core.CapabilityKindTool},
		{Name: "file_delete", Kind: core.CapabilityKindTool},
		{Name: "file_write", Kind: core.CapabilityKindTool},
		{Name: "search_find_similar", Kind: core.CapabilityKindTool},
		{Name: "search_semantic", Kind: core.CapabilityKindTool},
		{Name: "git_diff", Kind: core.CapabilityKindTool},
		{Name: "git_history", Kind: core.CapabilityKindTool},
		{Name: "git_blame", Kind: core.CapabilityKindTool},
		{Name: "query_ast", Kind: core.CapabilityKindTool},
	}
}

func defaultIgnoredGeneratedChanges() []string {
	return []string{
		"relurpify_cfg/sessions/**",
		"relurpify_cfg/memory/**",
		"relurpify_cfg/memory/ast_index/**",
		"**/target/**",
		"**/node_modules/**",
		"**/__pycache__/**",
		"**/.pytest_cache/**",
		"**/*.pyc",
		"**/*.pyo",
		"**/.mypy_cache/**",
		"**/coverage/**",
		"**/.coverage",
	}
}

func applyAgentTestCapabilityDefaults(registry *capability.Registry, allowedCapabilities []core.CapabilitySelector) {
	if registry == nil {
		return
	}
	_ = registerCapabilityAlias(registry, "read_file", "file_read")
	_ = registerCapabilityAlias(registry, "write_file", "file_write")
	registry.RestrictToCapabilities(uniqueCapabilitySelectors(allowedCapabilities))
}

func mergeCapabilitySelectors(base, extra []core.CapabilitySelector) []core.CapabilitySelector {
	if len(extra) == 0 {
		return append([]core.CapabilitySelector{}, base...)
	}
	return uniqueCapabilitySelectors(append(append([]core.CapabilitySelector{}, base...), extra...))
}

func uniqueCapabilitySelectors(input []core.CapabilitySelector) []core.CapabilitySelector {
	if len(input) == 0 {
		return nil
	}
	out := make([]core.CapabilitySelector, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, selector := range input {
		key := selector.ID + "|" + selector.Name + "|" + string(selector.Kind) + "|" +
			strings.Join(selector.Tags, ",") + "|" + strings.Join(selector.ExcludeTags, ",")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, selector)
	}
	return out
}

func registerCapabilityAlias(registry *capability.Registry, alias, target string) error {
	if registry == nil || alias == "" || target == "" {
		return nil
	}
	if _, ok := registry.Get(alias); ok {
		return nil
	}
	targetTool, ok := registry.Get(target)
	if !ok {
		return nil
	}
	return registry.Register(&aliasTool{
		alias:  alias,
		target: targetTool,
	})
}

type aliasTool struct {
	alias  string
	target core.Tool
}

func (t *aliasTool) Name() string        { return t.alias }
func (t *aliasTool) Description() string { return "Alias for " + t.target.Name() }
func (t *aliasTool) Category() string    { return t.target.Category() }
func (t *aliasTool) Parameters() []core.ToolParameter {
	return t.target.Parameters()
}
func (t *aliasTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	return t.target.Execute(ctx, state, args)
}
func (t *aliasTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return t.target.IsAvailable(ctx, state)
}
func (t *aliasTool) Permissions() core.ToolPermissions {
	return t.target.Permissions()
}
func (t *aliasTool) Tags() []string { return t.target.Tags() }

func instantiateAgentByName(workspace, name string, model core.LanguageModel, tools *capability.Registry, mem memory.MemoryStore, indexManager *ast.IndexManager) graph.Agent {
	paths := config.New(workspace)
	checkpointPath := paths.CheckpointsDir()
	workflowStatePath := paths.WorkflowStateFile()
	switch strings.ToLower(name) {
	case "planner":
		return &agents.PlannerAgent{Model: model, Tools: tools, Memory: mem}
	case "react":
		return &agents.ReActAgent{
			Model:          model,
			Tools:          tools,
			Memory:         mem,
			IndexManager:   indexManager,
			CheckpointPath: checkpointPath,
		}
	case "reflection":
		return &agents.ReflectionAgent{
			Reviewer: model,
			Delegate: &agents.CodingAgent{
				Model:             model,
				Tools:             tools,
				Memory:            mem,
				IndexManager:      indexManager,
				CheckpointPath:    checkpointPath,
				WorkflowStatePath: workflowStatePath,
			},
		}
	case "eternal":
		return &agents.EternalAgent{Model: model}
	default:
		return &agents.CodingAgent{
			Model:             model,
			Tools:             tools,
			Memory:            mem,
			IndexManager:      indexManager,
			CheckpointPath:    checkpointPath,
			WorkflowStatePath: workflowStatePath,
		}
	}
}
