package agenttest

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	appruntime "github.com/lexcodex/relurpify/app/relurpish/runtime"
	"github.com/lexcodex/relurpify/framework/agentenv"
	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/config"
	contractpkg "github.com/lexcodex/relurpify/framework/contract"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/policybundle"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
	namedfactory "github.com/lexcodex/relurpify/named/factory"
)

var bootstrapAgentRuntime = appruntime.BootstrapAgentRuntime

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

	// NEW: ToolsAvailable preflight check - fails fast if tool not in registry
	if len(req.ToolsAvailable) > 0 {
		reg := extractCapabilityRegistry(agent)
		if reg == nil {
			return "agent has no capability registry", true
		}
		for _, tool := range req.ToolsAvailable {
			tool = strings.TrimSpace(tool)
			if tool == "" {
				continue
			}
			if _, ok := reg.Get(tool); !ok {
				return fmt.Sprintf("tool %s not available in registry", tool), true
			}
		}
	}

	return "", false
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

func buildAgent(ctx context.Context, workspace, manifestPath, agentName string, agentSpec *core.AgentRuntimeSpec, model core.LanguageModel, telemetry core.Telemetry, opts RunOptions, extraEnv []string, allowedCapabilities []core.CapabilitySelector, c CaseSpec, mem memory.MemoryStore) (graph.Agent, *core.Context, error) {
	executionAgentName := resolveExecutionAgentName(agentName, c)
	agentManifest, err := manifest.LoadAgentManifest(manifestPath)
	if err != nil {
		return nil, nil, err
	}
	if agentSpec == nil {
		agentSpec = contractpkg.ApplyManifestDefaults(agentManifest.Spec.Agent, agentManifest.Spec.Defaults)
		if agentSpec == nil {
			agentSpec = &core.AgentRuntimeSpec{}
		}
	}
	bootstrapSpec := core.MergeAgentSpecs(agentSpec, core.AgentSpecOverlay{
		AllowedCapabilities: uniqueCapabilitySelectors(allowedCapabilities),
	})

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
			Backend:      appruntime.DefaultConfig().SandboxBackend,
			Sandbox:      appruntime.DefaultConfig().Sandbox,
			AuditLimit:   512,
			BaseFS:       workspace,
			HITLTimeout:  30 * time.Second,
		})
		if err != nil {
			return nil, nil, err
		}
		runner, err = fsandbox.NewCommandRunner(reg.Manifest, reg.Runtime, workspace)
		if err != nil {
			return nil, nil, err
		}
		permMgr = reg.Permissions
	} else {
		runner = fsandbox.NewLocalCommandRunner(workspace, extraEnv)
	}

	maxIterations := resolveCaseMaxIterations(opts, c)
	boot, err := bootstrapAgentRuntime(workspace, appruntime.AgentBootstrapOptions{
		Context:             ctx,
		AgentID:             agentManifest.Metadata.Name,
		AgentName:           executionAgentName,
		ConfigName:          executionAgentName,
		AgentsDir:           config.New(workspace).AgentsDir(),
		AgentSpec:           bootstrapSpec,
		Manifest:            agentManifest,
		PermissionManager:   permMgr,
		Runner:              runner,
		Model:               model,
		Memory:              mem,
		Telemetry:           telemetry,
		InferenceModel:      firstNonEmpty(opts.ModelOverride, agentSpec.Model.Name),
		SkipASTIndex:        opts.SkipASTIndex,
		AllowedCapabilities: uniqueCapabilitySelectors(allowedCapabilities),
		MaxIterations:       maxIterations,
		DebugLLM:            opts.DebugLLM,
		DebugAgent:          opts.DebugAgent,
	})
	if err != nil {
		return nil, nil, err
	}
	registry := boot.Registry
	paths := config.New(workspace)
	registry.UseSandboxScope(fsandbox.NewFileScopePolicy(workspace, paths.GovernanceRoots(paths.ManifestFile(), paths.ConfigFile(), paths.NexusConfigFile(), paths.PolicyRulesFile(), paths.ModelProfilesDir())))
	indexManager := boot.IndexManager
	searchEngine := boot.SearchEngine
	compiledPolicy := boot.CompiledPolicy
	if compiledPolicy == nil {
		contract := boot.Contract
		if contract == nil {
			contract = &contractpkg.EffectiveAgentContract{
				AgentID:   agentManifest.Metadata.Name,
				AgentSpec: agentSpec,
			}
		} else if contract.AgentSpec == nil {
			contract.AgentSpec = agentSpec
		}
		compiledPolicy, err = policybundle.BuildFromSpec(contract.AgentID, contract.AgentSpec, permMgr)
		if err != nil {
			return nil, nil, err
		}
	}
	registry.SetPolicyEngine(compiledPolicy.Engine)
	applyAgentTestCapabilityDefaults(registry, allowedCapabilities)
	pregrantAgentTestCapabilities(permMgr, agentManifest.Metadata.Name, executionAgentName, registry)

	// NEW: Wrap registry with tool injection interceptor if overrides are configured
	if len(c.Setup.ToolOverrides) > 0 {
		registry = WrapRegistryWithInterceptor(registry, c.Setup.ToolOverrides)
	}

	// NEW: Add precheck to block disabled tools from being invoked
	if len(c.Requires.ToolsDisable) > 0 {
		registry.AddPrecheck(&disabledToolPrecheck{disabled: c.Requires.ToolsDisable})
	}

	mem = boot.Memory

	env := boot.Environment
	env.Model = model
	env.Registry = registry
	env.Memory = mem
	env.IndexManager = indexManager
	env.SearchEngine = searchEngine
	agent := instantiateAgentByName(workspace, executionAgentName, env)
	if err := applyCaseControlFlowOverride(agent, c); err != nil {
		return nil, nil, err
	}
	return agent, core.NewContext(), nil
}

func resolveExecutionAgentName(agentName string, c CaseSpec) string {
	name := strings.ToLower(strings.TrimSpace(agentName))
	if name != "coding" {
		return agentName
	}
	mode := ""
	workflowID := ""
	if c.Context != nil {
		if raw, ok := c.Context["mode"]; ok {
			mode = strings.ToLower(strings.TrimSpace(fmt.Sprint(raw)))
		}
		if raw, ok := c.Context["workflow_id"]; ok {
			workflowID = strings.TrimSpace(fmt.Sprint(raw))
		}
	}
	switch mode {
	case "architect":
		if workflowID == "" {
			return "architect"
		}
		return "coding"
	case "ask", "debug", "docs", "code":
		return "coding"
	}
	return agentName
}

func resolveCaseMaxIterations(opts RunOptions, c CaseSpec) int {
	maxIterations := opts.MaxIterations
	if c.Overrides.MaxIterations > 0 {
		maxIterations = c.Overrides.MaxIterations
	}
	if maxIterations <= 0 {
		maxIterations = 8
	}
	return maxIterations
}

func resolveCaseTimeout(opts RunOptions, suite *Suite, c CaseSpec) (time.Duration, error) {
	if timeout, err := parseCaseTimeout(c.Timeout); err != nil {
		return 0, err
	} else if timeout > 0 {
		return timeout, nil
	}
	if suite != nil {
		if timeout, err := parseCaseTimeout(suite.Spec.Execution.Timeout); err != nil {
			return 0, err
		} else if timeout > 0 {
			return timeout, nil
		}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	return timeout, nil
}

func resolveBootstrapTimeout(opts RunOptions, c CaseSpec) time.Duration {
	if timeout, err := parseCaseTimeout(c.Overrides.BootstrapTimeout); err == nil && timeout > 0 {
		return timeout
	}
	timeout := opts.BootstrapTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return timeout
}

func applyCaseControlFlowOverride(_ graph.Agent, c CaseSpec) error {
	flow := strings.TrimSpace(c.Overrides.ControlFlow)
	if flow == "" {
		return nil
	}
	// Control flow overrides are no longer supported via CodingAgent modes.
	// Named agents define their own control scheme. Return an error so test
	// authors are aware the override field has no effect.
	return fmt.Errorf("control_flow override %q not supported: named agents manage their own control scheme", flow)
}

func defaultAgenttestAllowedCapabilities() []core.CapabilitySelector {
	return []core.CapabilitySelector{
		{ID: "agent:architect", Kind: core.CapabilityKindTool},
		{ID: "agent:blackboard", Kind: core.CapabilityKindTool},
		{ID: "agent:chainer", Kind: core.CapabilityKindTool},
		{ID: "agent:htn", Kind: core.CapabilityKindTool},
		{ID: "agent:pipeline", Kind: core.CapabilityKindTool},
		{ID: "agent:planner", Kind: core.CapabilityKindTool},
		{ID: "agent:react", Kind: core.CapabilityKindTool},
		{ID: "agent:reflection", Kind: core.CapabilityKindTool},
		{ID: "agent:rewoo", Kind: core.CapabilityKindTool},
		{Name: "browser", Kind: core.CapabilityKindTool},
		{Name: "exec_run_build", Kind: core.CapabilityKindTool},
		{Name: "exec_run_code", Kind: core.CapabilityKindTool},
		{Name: "exec_run_linter", Kind: core.CapabilityKindTool},
		{Name: "exec_run_tests", Kind: core.CapabilityKindTool},
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
		{Name: "go_build", Kind: core.CapabilityKindTool},
		{Name: "go_test", Kind: core.CapabilityKindTool},
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

func pregrantAgentTestCapabilities(manager *fauthorization.PermissionManager, agentID, executionAgentName string, registry *capability.Registry) {
	if manager == nil || registry == nil {
		return
	}
	resources := uniqueStrings([]string{
		strings.TrimSpace(agentID),
		strings.TrimSpace(executionAgentName),
		"coding",
		"react",
		"planner",
		"reflection",
		"architect",
	})
	for _, desc := range registry.CallableCapabilities() {
		actions := []string{"capability:" + desc.ID}
		if name := strings.TrimSpace(desc.Name); name != "" && name != desc.ID {
			actions = append(actions, "capability:"+name)
		}
		for _, action := range actions {
			for _, resource := range resources {
				if resource == "" {
					continue
				}
				manager.GrantPermission(core.PermissionDescriptor{
					Type:         core.PermissionTypeCapability,
					Action:       action,
					Resource:     resource,
					RequiresHITL: true,
				}, "agenttest-auto", fauthorization.GrantScopeSession, 0)
			}
		}
	}
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

// disabledToolPrecheck blocks invocation of disabled tools
type disabledToolPrecheck struct {
	disabled []string
}

func (d *disabledToolPrecheck) Check(descriptor core.CapabilityDescriptor, args map[string]any) error {
	for _, name := range d.disabled {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		// Check if this descriptor matches the disabled tool name (by ID or Name)
		if strings.EqualFold(descriptor.ID, name) || strings.EqualFold(descriptor.Name, name) {
			return fmt.Errorf("tool %s is disabled for this test case", name)
		}
	}
	return nil
}

func instantiateAgentByName(workspace, name string, env agentenv.AgentEnvironment) graph.Agent {
	return namedfactory.InstantiateByName(workspace, name, env)
}
