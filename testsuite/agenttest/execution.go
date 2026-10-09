package agenttest

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/app/envcomposition"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	aconvert "codeburg.org/lexbit/relurpify/capability/agentspec/convert"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/memory"
	"codeburg.org/lexbit/relurpify/context/persistence"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/named/euclo"
	"codeburg.org/lexbit/relurpify/named/euclo/euclocontract"
	euclopolicy "codeburg.org/lexbit/relurpify/named/euclo/policy"
	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	cfgsecurity "codeburg.org/lexbit/relurpify/userconfig/config/security"
)

// PreparedRunExecutor is the bridge between the agenttest preparation layer
// and the Euclo agent runtime. It constructs the full dependency chain
// (security, capability, knowledge, model, agent) and executes the case.
type PreparedRunExecutor struct {
	security   *envcomposition.SecurityRuntime
	capability *envcomposition.CapabilityRuntime
	knowledge  *envcomposition.KnowledgeRuntime
	model      *envcomposition.ModelRuntime
	telemetry  telemetry.Telemetry
	agent      *euclo.Agent

	// recorder captures every telemetry event in memory for the duration of the
	// run so the case report and the OSB evaluators are derived from real
	// telemetry (FR-8). telemetryFile is the durable JSONL mirror; it is nil
	// when the descriptor has no telemetry directory.
	recorder      *recordingTelemetrySink
	telemetryFile *telemetry.JSONFileTelemetry
	// knowledgeBridge forwards knowledge.EventBus chunk signals into the
	// recorder for the duration of the run (FR-14).
	knowledgeBridge *knowledge.EventBusTelemetryBridge

	// Security inputs resolved from the derived workspace during buildSecurity,
	// reused by buildCapability. They are populated only when the executor
	// builds the runtime from the workspace (i.e. no runner override).
	securityBundle *cfgsecurity.Bundle
	agentSpec      *agentspec.AgentRuntimeSpec
	permManager    *fauthorization.PermissionManager
	hitlBroker     euclopolicy.HITLBroker

	runnerOverride sandbox.CommandRunner       // test seam; nil in production
	agentOverride  agentgraph.WorkflowExecutor // test seam; nil in production
}

func (e *PreparedRunExecutor) Execute(ctx context.Context, desc *PreparedRunDescriptor, out io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := desc.Validate(); err != nil {
		return fmt.Errorf("validate descriptor: %w", err)
	}
	if err := validateRecordingPlan(desc); err != nil {
		return fmt.Errorf("recording plan: %w", err)
	}
	defer e.cleanup()
	startedAt := time.Now().UTC()

	if err := e.buildSecurity(ctx, desc); err != nil {
		return fmt.Errorf("security: %w", err)
	}
	if err := e.buildCapability(ctx, desc); err != nil {
		return fmt.Errorf("capability: %w", err)
	}
	tel := e.buildTelemetry(desc)
	e.telemetry = tel
	// Phase 6 silent-domain bridging: attach the recording sink to the sandbox
	// chain and the knowledge trigger so denials, executions, and compilation
	// jobs produce real telemetry inside e2e cases (FR-12/FR-15).
	if e.security != nil && e.security.Runner != nil {
		e.security.Runner.SetTelemetry(tel)
	}
	if err := e.buildKnowledge(); err != nil {
		return fmt.Errorf("knowledge: %w", err)
	}
	if err := e.buildModel(ctx, desc); err != nil {
		return fmt.Errorf("model: %w", err)
	}
	deps := e.assembleDeps(desc, tel)
	if err := e.createAgent(deps); err != nil {
		return fmt.Errorf("agent: %w", err)
	}
	task := &execution.Task{
		ID:          desc.RunID,
		Type:        inferTaskType(desc),
		Instruction: desc.Instruction,
	}
	env := contextdata.NewEnvelope(desc.RunID, desc.RunID)
	result, attempts, triggeredBy, execErr := e.executeWithRetry(ctx, desc, task, env, out)
	verifications := e.runVerification(ctx, desc)
	report := e.buildCaseReport(desc, result, execErr, attempts, triggeredBy, verifications, startedAt)
	if e.recorder != nil {
		events := e.recorder.Events()
		applyRecordedTelemetry(&report, events)
		writeCaseTelemetryArtifacts(desc, &report, events)
	}
	if reportPath := caseReportPath(desc); reportPath != "" {
		if werr := WriteCaseReport(reportPath, report); werr != nil {
			_ = werr
		}
	}
	if report.Output != "" {
		_, _ = io.Copy(out, strings.NewReader(report.Output))
	}
	if execErr != nil && report.FailureKind == "infra" {
		return fmt.Errorf("infrastructure failure: %w", execErr)
	}
	return nil
}

func (e *PreparedRunExecutor) buildSecurity(ctx context.Context, desc *PreparedRunDescriptor) error {
	workspace := firstNonEmpty(desc.DerivedWorkspaceRoot, desc.WorkspaceRoot)

	// A runner override (test seam) bypasses the full security assembly: the
	// caller is responsible for the runner and its policy. This keeps the
	// hermetic unit tests dependency-free.
	if e.runnerOverride != nil {
		sec, err := envcomposition.BuildSecurityRuntime(ctx, envcomposition.SecurityRuntimeInput{
			Context:        ctx,
			Workspace:      workspace,
			SandboxBackend: desc.SandboxBackend,
			AgentID:        desc.AgentName,
			ExistingRunner: e.runnerOverride,
		})
		if err != nil {
			return err
		}
		e.security = sec
		// Test seam: no agent registration runs on this path, so supply an
		// auto-approving broker to keep the graph buildable. The production
		// path always uses the registration broker.
		broker := fauthorization.NewHITLBroker(0, nil)
		broker.AutoApprove = true
		e.hitlBroker = broker
		return nil
	}

	// Production path: resolve the security bundle, agent spec and permission
	// manager from the derived workspace, mirroring the relurpish boot. Without
	// this buildRunnerImpl fails with "security bundle required to build
	// sandbox runner" whenever the sandbox is enabled (the default).
	bundle, err := cfgsecurity.LoadBundle(workspace, config.StrictDecode)
	if err != nil {
		return fmt.Errorf("load security bundle: %w", err)
	}
	contract, err := config.OverlaySecurityBundle(euclocontract.DefaultContract(), bundle)
	if err != nil {
		return fmt.Errorf("overlay security bundle: %w", err)
	}
	agentSpec := aconvert.ConvertAgentSpec(contract.AgentSpec)
	docSnapshot := &config.DocumentSnapshot{
		Document: &config.Document{
			APIVersion: "relurpify.io/v1",
			Kind:       "AgentManifest",
			Metadata:   config.DocumentMetadata{Name: contract.AgentID},
		},
		Fingerprint: config.ContractFingerprint(contract, workspace),
		LoadedAt:    time.Now().UTC(),
	}
	registration, err := fauthorization.RegisterAgent(ctx, fauthorization.RuntimeConfig{
		DocumentSnapshot: docSnapshot,
		AgentSpec:        contract.AgentSpec,
		Permissions:      contract.Permissions,
		Security: fauthorization.SandboxSecurity{
			RunAsUser:       contract.Security.RunAsUser,
			ReadOnlyRoot:    contract.Security.ReadOnlyRoot,
			NoNewPrivileges: contract.Security.NoNewPrivileges,
		},
		ProtectedPaths: bundle.Sandbox.ProtectedPaths,
		Backend:        desc.SandboxBackend,
		BackendFactory: envcomposition.NewSandboxBackendFactory(),
		BaseFS:         workspace,
		StateDir:       config.DefaultWorkspaceStateDir(workspace),
		WorkspaceID:    filepath.Base(workspace),
		AgentName:      contract.AgentID,
	})
	if err != nil {
		return fmt.Errorf("register agent: %w", err)
	}

	sec, err := envcomposition.BuildSecurityRuntime(ctx, envcomposition.SecurityRuntimeInput{
		Context:           ctx,
		Workspace:         workspace,
		SandboxBackend:    desc.SandboxBackend,
		AgentID:           registration.ID,
		AgentSpec:         agentSpec,
		SecurityBundle:    bundle,
		Security:          contract.Security,
		PermissionManager: registration.Permissions,
	})
	if err != nil {
		return err
	}
	e.security = sec
	e.securityBundle = bundle
	e.agentSpec = agentSpec
	e.permManager = registration.Permissions
	e.hitlBroker = registration.HITL
	return nil
}

func (e *PreparedRunExecutor) buildCapability(ctx context.Context, desc *PreparedRunDescriptor) error {
	workspace := firstNonEmpty(desc.DerivedWorkspaceRoot, desc.WorkspaceRoot)
	opts := envcomposition.CapabilityRuntimeOptions{
		Context:      ctx,
		AgentID:      desc.AgentName,
		SkipASTIndex: desc.SkipASTIndex,
	}
	// When the security stage resolved an agent spec / permission manager /
	// protected paths from the workspace, feed them through so capability
	// admission is consistent with the production boot.
	if e.agentSpec != nil {
		opts.AgentSpec = e.agentSpec
	}
	if e.permManager != nil {
		opts.PermissionManager = e.permManager
	}
	if e.securityBundle != nil {
		opts.ProtectedPaths = e.securityBundle.Sandbox.ProtectedPaths
	}
	cap, err := envcomposition.BuildCapabilityRuntime(ctx, workspace, e.security.Runner, opts)
	if err != nil {
		return err
	}
	e.capability = cap
	return nil
}

func (e *PreparedRunExecutor) buildKnowledge() error {
	kn, err := envcomposition.BuildKnowledgeRuntime(envcomposition.KnowledgeRuntimeInput{
		GraphDB: e.capability.IndexManager.GraphDB,
		Index:   e.capability.IndexManager,
	})
	if err != nil {
		return err
	}
	if e.telemetry != nil {
		if kn.StreamTrigger != nil {
			kn.StreamTrigger.SetTelemetry(e.telemetry)
		}
		if kn.KnowledgeEvents != nil {
			kn.KnowledgeEvents.SetTelemetry(e.telemetry)
			e.knowledgeBridge = knowledge.NewEventBusTelemetryBridge(kn.KnowledgeEvents, e.telemetry)
		}
	}
	e.knowledge = kn
	return nil
}

func (e *PreparedRunExecutor) buildModel(ctx context.Context, desc *PreparedRunDescriptor) error {
	var tapePath string
	switch strings.TrimSpace(desc.RecordingMode) {
	case "", "off":
		tapePath = ""
	case "record", "replay":
		tapePath = desc.TapePath
	}
	kind := desc.BackendFamily
	if strings.TrimSpace(desc.RecordingMode) == "replay" {
		kind = "tape"
	}
	mr, err := envcomposition.BuildModelRuntime(envcomposition.ModelRuntimeInput{
		Provider:          desc.BackendProvider,
		Kind:              kind,
		Endpoint:          desc.BackendEndpoint,
		ModelName:         desc.ModelName,
		TapePath:          tapePath,
		NativeToolCalling: false,
		Timeout:           30 * time.Second,
		Secrets:           llm.ProviderSecrets{},
		Profile:           nil,
	})
	if err != nil {
		return err
	}
	e.model = mr
	return nil
}

func (e *PreparedRunExecutor) buildTelemetry(desc *PreparedRunDescriptor) telemetry.Telemetry {
	e.recorder = newRecordingTelemetrySink()
	sinks := []telemetry.Telemetry{e.recorder}
	if file := e.openTelemetryFile(desc); file != nil {
		e.telemetryFile = file
		sinks = append(sinks, file)
	}
	if len(sinks) == 1 {
		return e.recorder
	}
	return telemetry.MultiplexTelemetry{Sinks: sinks}
}

// openTelemetryFile opens the durable JSONL mirror for the run. It is
// best-effort: without a telemetry directory the recording sink is still the
// source of truth, and an open failure must never abort the run (NFR-3).
func (e *PreparedRunExecutor) openTelemetryFile(desc *PreparedRunDescriptor) *telemetry.JSONFileTelemetry {
	if desc == nil {
		return nil
	}
	dir := firstNonEmpty(desc.ExecutionTelemetryDir, desc.TelemetryDir)
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	if err := fs.MkdirAllSecure(dir); err != nil {
		return nil
	}
	file, err := telemetry.NewJSONFileTelemetry(filepath.Join(dir, "agenttest.jsonl"))
	if err != nil {
		return nil
	}
	return file
}

func (e *PreparedRunExecutor) assembleDeps(desc *PreparedRunDescriptor, tel telemetry.Telemetry) *paradigm.Deps {
	cfg := &execution.Config{
		Name:          desc.AgentName,
		Model:         desc.ModelName,
		MaxIterations: desc.MaxIterations,
		Workspace:     desc.DerivedWorkspaceRoot,
		Telemetry:     tel,
	}
	modelTel := modelTelemetryAdapter{inner: tel}
	return &paradigm.Deps{
		Config:         cfg,
		Model:          e.model.ModelFactory(modelTel, false),
		Registry:       e.capability.Registry,
		CommandRunner:  e.security.Runner,
		CommandPolicy:  e.security.CommandPolicy,
		WorkingMemory:  memory.NewWorkingMemoryStore(),
		IndexManager:   e.capability.IndexManager,
		SearchEngine:   e.capability.SearchEngine,
		StreamTrigger:  e.knowledge.StreamTrigger,
		OutputIngester: knowledge.NewOutputIngester(e.knowledge.KnowledgeStore, e.knowledge.KnowledgeEvents),
		IngestOutputs:  true,
		PromptRegistry: prompt.NewRegistry(),
		AgentLifecycle: nil,
		Telemetry:      tel,
	}
}

func (e *PreparedRunExecutor) createAgent(deps *paradigm.Deps) error {
	agent := euclo.New(
		deps,
		euclo.WithCheckpointRepository(deps.AgentLifecycle),
		euclo.WithHITLBroker(e.hitlBroker),
		euclo.WithLifecycleRepository(e.lifecycleRepository()),
	)
	if err := agent.Initialize(nil); err != nil {
		return err
	}
	e.agent = agent
	return nil
}

// lifecycleRepository exposes the workspace graph database as the durable
// lifecycle repository that receives Selection Decision Records (D11). Without
// one, agenttest dispatches would silently skip the phase's provenance
// contract (AC-10). The engine is shared with the capability index and closed
// by cleanup; the repository is a thin immutable adapter.
func (e *PreparedRunExecutor) lifecycleRepository() contextports.LifecycleRepository {
	if e == nil || e.capability == nil || e.capability.IndexManager == nil || e.capability.IndexManager.GraphDB == nil {
		return nil
	}
	return persistence.NewLifecycleRepository(e.capability.IndexManager.GraphDB)
}

func (e *PreparedRunExecutor) currentExecutor() agentgraph.WorkflowExecutor {
	if e.agentOverride != nil {
		return e.agentOverride
	}
	return e.agent
}

// Backend reset strategies (D13).
const (
	resetStrategyNone   = "none"
	resetStrategyModel  = "model"
	resetStrategyServer = "server"
)

func (e *PreparedRunExecutor) executeWithRetry(ctx context.Context, desc *PreparedRunDescriptor, task *execution.Task, env *contextdata.Envelope, out io.Writer) (*execution.Result, int, []string, error) {
	var (
		result      *execution.Result
		err         error
		retryCount  int
		triggeredBy []string
	)
	exec := e.currentExecutor()
	for attempt := 0; attempt <= desc.MaxRetries; attempt++ {
		result, err = exec.Execute(ctx, task, env)
		if err == nil {
			break
		}
		if attempt >= desc.MaxRetries {
			break
		}
		if len(desc.resetPatterns) > 0 {
			// Pattern mode (D13): a retry happens exactly when a configured
			// regex matches the error, and a non-matching error does not spend
			// the retry budget or trigger a reset.
			if !anyResetPatternMatches(desc.resetPatterns, err.Error()) {
				break
			}
		} else {
			// Legacy mode: no patterns configured, so the reset strategy alone
			// gates retries (unchanged pre-D13 behavior).
			strategy := strings.TrimSpace(desc.BackendResetStrategy)
			if strategy == "" || strategy == resetStrategyNone {
				break
			}
		}
		triggeredBy = append(triggeredBy, err.Error())
		retryCount++
		if rerr := e.resetBackend(ctx, desc); rerr != nil {
			log.Printf("agenttest: backend reset failed: %v", rerr)
		}
	}
	return result, retryCount + 1, triggeredBy, err
}

// anyResetPatternMatches reports whether any compiled trigger pattern matches
// the execution error text.
func anyResetPatternMatches(patterns []*regexp.Regexp, message string) bool {
	for _, re := range patterns {
		if re.MatchString(message) {
			return true
		}
	}
	return false
}

func (e *PreparedRunExecutor) resetBackend(ctx context.Context, desc *PreparedRunDescriptor) error {
	strategy := strings.TrimSpace(desc.BackendResetStrategy)
	if strategy == "" && len(desc.resetPatterns) > 0 {
		// Patterns are configured but no strategy was declared: a matched
		// trigger is meant to reset, so default to the model strategy (D13).
		strategy = resetStrategyModel
	}
	switch strategy {
	case "", resetStrategyNone:
		return nil
	case resetStrategyModel:
		return e.model.Backend.Reset(ctx, resetStrategyModel)
	case resetStrategyServer:
		if strings.TrimSpace(desc.BackendService) == "" {
			return fmt.Errorf("server reset requires backend_service")
		}
		cmd := exec.CommandContext(ctx, "systemctl", "restart", desc.BackendService)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("restart %s: %w (%s)", desc.BackendService, err, out)
		}
		return nil
	default:
		return fmt.Errorf("unknown reset strategy: %s", strategy)
	}
}

func (e *PreparedRunExecutor) cleanup() {
	if e.knowledgeBridge != nil {
		e.knowledgeBridge.Close()
		e.knowledgeBridge = nil
	}
	if e.telemetryFile != nil {
		_ = e.telemetryFile.Close()
		e.telemetryFile = nil
	}
	if e.model != nil && e.model.Backend != nil {
		_ = e.model.Backend.Close()
	}
	if e.capability != nil && e.capability.IndexManager != nil {
		_ = e.capability.IndexManager.Close(context.Background())
	}
}

func (e *PreparedRunExecutor) runVerification(ctx context.Context, desc *PreparedRunDescriptor) []AssertionResult {
	if len(desc.Verification.Steps) == 0 && strings.TrimSpace(desc.Verification.Script) == "" {
		return nil
	}
	spec := verifySpecFromContract(desc.Verification)
	workspace := firstNonEmpty(desc.DerivedWorkspaceRoot, desc.WorkspaceRoot)
	return runVerificationSteps(ctx, spec, workspace, e.security.Runner)
}

func verifySpecFromContract(contract PreparedVerificationContract) VerifySpec {
	steps := make([]VerifyStepSpec, 0, len(contract.Steps))
	for _, s := range contract.Steps {
		steps = append(steps, VerifyStepSpec{
			Tool:              s.Tool,
			Args:              s.Args,
			ContinueOnFailure: s.ContinueOnFailure,
		})
	}
	return VerifySpec{
		Steps:  steps,
		Script: contract.Script,
	}
}

func (e *PreparedRunExecutor) buildCaseReport(desc *PreparedRunDescriptor, result *execution.Result, execErr error, attempts int, triggeredBy []string, verifications []AssertionResult, startedAt time.Time) CaseReport {
	report := CaseReport{
		Name:             desc.CaseName,
		Model:            desc.ModelName,
		Provider:         desc.BackendProvider,
		Endpoint:         desc.BackendEndpoint,
		RecordingMode:    desc.RecordingMode,
		TapePath:         desc.TapePath,
		ArtifactsDir:     caseArtifactsDir(desc),
		StartedAt:        startedAt,
		FinishedAt:       time.Now().UTC(),
		DurationMS:       time.Since(startedAt).Milliseconds(),
		Attempts:         attempts,
		RetryCount:       attempts - 1,
		RetryTriggeredBy: triggeredBy,
		AssertionResults: verifications,
	}
	if result != nil {
		report.Success = result.Success
		report.Error = result.Error
	}
	if execErr != nil {
		report.Success = false
		if report.Error == "" {
			report.Error = execErr.Error()
		}
		report.FailureKind = classifyFailure(execErr)
	}
	return report
}

type modelTelemetryAdapter struct {
	inner telemetry.Telemetry
}

func (a modelTelemetryAdapter) Emit(ctx context.Context, event any) {
	if a.inner == nil {
		return
	}
	if typed, ok := event.(telemetry.Event); ok {
		a.inner.Emit(typed)
	}
}

// WithRunnerOverride is a test seam for injecting a sandbox.CommandRunner
// so unit tests run without gVisor. NOT for production use.
func (e *PreparedRunExecutor) WithRunnerOverride(r sandbox.CommandRunner) *PreparedRunExecutor {
	e.runnerOverride = r
	return e
}

// WithAgentOverride is a test seam for injecting a fake agentgraph.WorkflowExecutor
// so retry behaviour can be tested without a real agent.
func (e *PreparedRunExecutor) WithAgentOverride(a agentgraph.WorkflowExecutor) *PreparedRunExecutor {
	e.agentOverride = a
	return e
}

func inferTaskType(desc *PreparedRunDescriptor) string {
	if len(desc.Verification.Steps) > 0 || len(desc.ExpectedArtifacts) > 0 {
		return "code_modification"
	}
	return "chat"
}

func classifyFailure(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline") {
		return "infra"
	}
	if strings.Contains(msg, "permission") || strings.Contains(msg, "access denied") {
		return "security"
	}
	return "assertion"
}

func validateRecordingPlan(desc *PreparedRunDescriptor) error {
	switch strings.TrimSpace(desc.RecordingMode) {
	case "", "off":
		return nil
	case "record":
		return nil
	case "replay":
		if strings.TrimSpace(desc.TapePath) == "" {
			return fmt.Errorf("replay mode requires tape_path")
		}
		if _, err := os.Stat(desc.TapePath); err != nil {
			return fmt.Errorf("replay tape unavailable at %q: %w", desc.TapePath, err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported recording_mode %q", desc.RecordingMode)
	}
}
