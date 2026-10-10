package prep

import (
	"context"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/app/envcomposition"
	capfs "codeburg.org/lexbit/relurpify/capability/fs"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/memory"
	"codeburg.org/lexbit/relurpify/context/persistence"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/named/euclo"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
	"codeburg.org/lexbit/relurpify/testsuite/testsupport"
	"codeburg.org/lexbit/relurpify/userconfig/templates/embedfs"
)

// canonicalMarker in WorkspaceFiles requests the canonical 7 recipes be
// extracted from the embedded template into the temp workspace.
const canonicalMarker = "canonical"

// SeedChunk is one pre-ground graph entry: grounded through the real
// GroundingService before the agent runs, so seeds exercise the same write
// boundary (and canonical content-derived identity) as runtime captures.
type SeedChunk struct {
	Body string
	Kind knowledge.ChunkKind // zero value grounds as ChunkKindCapture
}

// DryRunConfig describes one hermetic dry run.
type DryRunConfig struct {
	Name           string
	WorkspaceFiles map[string]string // relative path → content; "canonical" extracts the embedded template recipes
	Instruction    string
	Turns          []testhelper.ModelTurn // scripted mind, consumed in order; last repeats
	Scripted       []*ScriptedCapability
	SeedChunks     []SeedChunk
	MaxIterations  int
	// DisableCompiler detaches the stream trigger for renderer-less paths;
	// the compiler is wired by default.
	DisableCompiler bool
}

// ChunkRecord is one grounded chunk in the post-run knowledge summary. The
// body is included: the dry-run report is the debugging tier where D-11's
// no-bodies redaction explicitly does not apply.
type ChunkRecord struct {
	ID            string
	ContentHash   string
	Body          string
	TokenEstimate int
}

// KnowledgeSummary reports the graph content the run left behind: seed chunks
// plus every chunk grounded through captures during the run (chunk.committed
// telemetry, cross-checked against the store).
type KnowledgeSummary struct {
	Chunks []ChunkRecord
}

// DryRunReport is the dry run's observation surface (D-6): everything a case
// asserts on lives here.
type DryRunReport struct {
	Dispatched       string // selected route (thought recipe ID or capability ID)
	DecidedBy        string // route resolution provenance
	FallbackTaken    bool
	ParadigmRuns     []string          // paradigms observed in telemetry, in first-seen order
	ModelInvocations int               // model calls the run made (diagnostic)
	CapabilityCalls  []CapabilityCall  // scripted capability invocations, in order
	ModelMessages    [][]model.Message // every prompt, verbatim (debugging tier; D-11 exception)
	StreamedSections []string          // rendered streamed-context sections, one per model call that carried one
	Selection        map[string]any    // route selection state from the envelope
	StateKeys        map[string]any    // asserted working-state keys (route fallback, outcome)
	Knowledge        KnowledgeSummary
	Events           []telemetry.Event // full telemetry trail (debugging tier)
	Errors           []string
	Success          bool
	Duration         time.Duration
}

// Run executes one hermetic dry run and returns its report. Hard composition
// failures (workspace setup, boot, initialize) return a non-nil error; run-level
// failures (execution errors, model starvation) land in report.Errors with
// Success=false.
func Run(ctx context.Context, cfg DryRunConfig) (*DryRunReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	report := &DryRunReport{Success: true}

	ws, err := os.MkdirTemp("", "prep-dryrun-*")
	if err != nil {
		return nil, fmt.Errorf("prep: temp workspace: %w", err)
	}
	defer os.RemoveAll(ws)
	if err := materializeWorkspace(ws, cfg.WorkspaceFiles); err != nil {
		return nil, fmt.Errorf("prep: workspace: %w", err)
	}
	if err := writeScriptedManifests(ws, cfg.Scripted); err != nil {
		return nil, fmt.Errorf("prep: capability manifests: %w", err)
	}

	sink := &recordingSink{}
	runner, err := testsupport.NewAuthorizedFakeRunner(testsupport.PermitAllPolicy())
	if err != nil {
		return nil, fmt.Errorf("prep: command runner: %w", err)
	}
	// No network egress and no exec authority: the fake runner denies by
	// default and the scripted capabilities are the only invocable surface the
	// scripted model can reach (D-7).
	capability, err := envcomposition.BuildCapabilityRuntime(ctx, ws, runner, envcomposition.CapabilityRuntimeOptions{
		Context:      ctx,
		AgentID:      "prep",
		SkipASTIndex: true,
	})
	if err != nil {
		return nil, fmt.Errorf("prep: capability runtime: %w", err)
	}
	if err := registerScripted(ctx, capability.Registry, cfg.Scripted); err != nil {
		return nil, fmt.Errorf("prep: capability registration: %w", err)
	}

	kn, err := envcomposition.BuildKnowledgeRuntime(envcomposition.KnowledgeRuntimeInput{
		GraphDB:       capability.IndexManager.GraphDB,
		Index:         capability.IndexManager,
		WorkspaceRoot: ws,
	})
	if err != nil {
		return nil, fmt.Errorf("prep: knowledge runtime: %w", err)
	}
	defer func() {
		kn.Close()
		_ = capability.IndexManager.Close(context.Background())
	}()
	kn.StreamTrigger.SetTelemetry(sink)
	kn.KnowledgeEvents.SetTelemetry(sink)
	knowledgeBridge := knowledge.NewEventBusTelemetryBridge(kn.KnowledgeEvents, sink)
	defer knowledgeBridge.Close()

	for i, seed := range cfg.SeedChunks {
		ground, err := kn.Grounding.Ground(ctx, []knowledge.GroundingItem{{
			Value:          seed.Body,
			TypeAnnotation: "text",
			Kind:           seed.Kind,
			Origin:         contextdata.OriginTool,
			StateKey:       fmt.Sprintf("prep.seed.%d", i),
			TaskID:         "prep-seed",
			WorkspaceID:    filepath.Base(ws),
		}})
		if err != nil {
			return nil, fmt.Errorf("prep: seed chunk %d: %w", i, err)
		}
		if len(ground.Grounded) != 1 {
			return nil, fmt.Errorf("prep: seed chunk %d not grounded: %+v", i, ground)
		}
	}

	model := testhelper.NewSequencedModel(cfg.Turns...)
	maxIterations := cfg.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 8
	}
	deps := &paradigm.Deps{
		Config: &execution.Config{
			Name:          "prep",
			Model:         "scripted",
			MaxIterations: maxIterations,
			Workspace:     ws,
			Telemetry:     sink,
		},
		Model:          model,
		Registry:       capability.Registry,
		CommandRunner:  runner,
		WorkingMemory:  memory.NewWorkingMemoryStore(),
		IndexManager:   capability.IndexManager,
		SearchEngine:   capability.SearchEngine,
		PromptRegistry: prompt.NewRegistry(),
		Telemetry:      sink,
	}
	if !cfg.DisableCompiler {
		deps.StreamTrigger = kn.StreamTrigger
	}
	deps = envcomposition.WireGrounding(deps, kn)

	agent := euclo.New(
		deps,
		euclo.WithHITLBroker(testsupport.NewAutoApprovingBroker()),
		euclo.WithLifecycleRepository(persistence.NewLifecycleRepository(capability.IndexManager.GraphDB)),
		euclo.WithInteractionResolver(testhelper.NewPermissiveResolver()),
	)
	if err := agent.Initialize(&execution.Config{Workspace: ws}); err != nil {
		return nil, fmt.Errorf("prep: initialize: %w", err)
	}

	task := &execution.Task{ID: "prep-run", Type: "chat", Instruction: cfg.Instruction}
	env := contextdata.NewEnvelope("prep-run", "prep-run")
	result, execErr := agent.Execute(ctx, task, env)
	if execErr != nil {
		report.Errors = append(report.Errors, execErr.Error())
		report.Success = false
	}
	if result != nil && result.Error != "" {
		report.Errors = append(report.Errors, result.Error)
		report.Success = false
	}
	// Model starvation surfaces as ErrTurnsExhausted inside execErr above: a
	// case that under-scripts its turns fails loudly here, not in a hang.

	collectRoute(report, env)
	report.ModelInvocations = model.InvocationCount()
	report.CapabilityCalls = collectCalls(cfg.Scripted)
	report.ParadigmRuns = collectParadigms(sink)
	report.ModelMessages = model.Messages()
	report.StreamedSections = extractStreamedSections(report.ModelMessages)
	report.Knowledge = collectKnowledge(sink, kn)
	report.Events = sink.Events()
	report.Duration = time.Since(started)
	return report, nil
}

func materializeWorkspace(ws string, files map[string]string) error {
	canonical := false
	for rel, content := range files {
		rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
		if rel == canonicalMarker {
			canonical = true
			continue
		}
		if strings.Contains(rel, "..") || filepath.IsAbs(rel) {
			return fmt.Errorf("workspace file %q escapes the workspace", rel)
		}
		target := filepath.Join(ws, filepath.FromSlash(rel))
		if err := capfs.MkdirAllSecure(filepath.Dir(target)); err != nil {
			return err
		}
		if err := capfs.WriteFileSecure(target, []byte(content)); err != nil {
			return err
		}
	}
	if !canonical {
		return nil
	}
	embed, err := iofs.Sub(embedfs.DefaultFS(), "workspace/euclo")
	if err != nil {
		return err
	}
	entries, err := iofs.ReadDir(embed, ".")
	if err != nil {
		return err
	}
	dest := filepath.Join(ws, "relurpify_cfg", "euclo")
	if err := capfs.MkdirAllSecure(dest); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".erpe") {
			continue
		}
		content, err := iofs.ReadFile(embed, entry.Name())
		if err != nil {
			return err
		}
		if err := capfs.WriteFileSecure(filepath.Join(dest, entry.Name()), content); err != nil {
			return err
		}
	}
	return nil
}

// writeScriptedManifests declares every scripted capability in the workspace tool
// manifest tree; the registry's admission policy admits only
// manifest-declared tools, so an undeclared one would be silently dropped at
// registration (deny-by-default registry invariant — the harness must pass
// through it, not bypass it).
func writeScriptedManifests(ws string, scripted []*ScriptedCapability) error {
	if len(scripted) == 0 {
		return nil
	}
	dir := filepath.Join(ws, "relurpify_cfg", "tools")
	if err := capfs.MkdirAllSecure(dir); err != nil {
		return err
	}
	var b strings.Builder
	for _, capability := range scripted {
		if capability == nil || capability.Name == "" {
			continue
		}
		b.Reset()
		fmt.Fprintf(&b, `schema: relurpify/tool/v1
name: %q
version: "1"
family: prep
description: prep dry-run scripted capability
execution:
  backend: go_native
  implementation: %q
capability:
  trust_class: builtin_trusted
  risk_class:
    - read_only
  effect_class:
    - pure
`, capability.Name, capability.Name)
		// one manifest per capability: the loader reads single-document YAML files
		name := sanitizeManifestFile(capability.Name)
		if err := capfs.WriteFileSecure(filepath.Join(dir, name+".tool.yaml"), []byte(b.String())); err != nil {
			return err
		}
	}
	return nil
}

// sanitizeManifestFile maps a capability ID to a safe manifest file name.
func sanitizeManifestFile(name string) string {
	replaced := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-', r == '.':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return '_'
		}
	}, name)
	return "prep_" + replaced
}

func collectRoute(report *DryRunReport, env *contextdata.Envelope) {
	report.Selection = map[string]any{}
	report.StateKeys = map[string]any{}
	if selection, ok := euclostate.GetRouteSelection(env); ok && selection != nil {
		report.Selection["route_kind"] = selection.RouteKind
		if selection.ThoughtRecipeID != "" {
			report.Dispatched = selection.ThoughtRecipeID
		} else {
			report.Dispatched = selection.CapabilityID
		}
	}
	report.DecidedBy = euclostate.GetRouteDecidedBy(env)
	report.FallbackTaken = euclostate.GetRouteFallbackTaken(env)
	report.StateKeys["fallback_id"] = euclostate.GetRouteFallbackID(env)
	report.StateKeys["route_outcome"] = euclostate.GetRouteOutcome(env)
	if kind, ok := euclostate.GetDispatchRouteKind(env); ok {
		report.StateKeys["dispatch_route_kind"] = kind
	}
}

func collectCalls(scripted []*ScriptedCapability) []CapabilityCall {
	var calls []CapabilityCall
	for _, capability := range scripted {
		calls = append(calls, capability.Calls()...)
	}
	return calls
}

func collectParadigms(sink *recordingSink) []string {
	seen := map[string]struct{}{}
	var order []string
	for _, ev := range sink.Events() {
		if ev.Metadata == nil {
			continue
		}
		name, ok := ev.Metadata["paradigm"].(string)
		if !ok || name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		order = append(order, name)
	}
	return order
}

func collectKnowledge(sink *recordingSink, kn *envcomposition.KnowledgeRuntime) KnowledgeSummary {
	summary := KnowledgeSummary{}
	seen := map[string]struct{}{}
	for _, ev := range sink.ofType(telemetry.EventChunkCommitted) {
		id, _ := ev.Metadata["chunk_id"].(string)
		if _, dup := seen[id]; id == "" || dup {
			continue
		}
		seen[id] = struct{}{}
		record := ChunkRecord{ID: id}
		if hash, ok := ev.Metadata["content_hash"].(string); ok {
			record.ContentHash = hash
		}
		if tokens, ok := ev.Metadata["token_estimate"].(int); ok {
			record.TokenEstimate = tokens
		}
		summary.Chunks = append(summary.Chunks, record)
	}
	// Cross-check committed events against the store: the report must not
	// claim a chunk the store cannot load.
	kept := summary.Chunks[:0]
	for _, record := range summary.Chunks {
		if chunk, ok, err := kn.KnowledgeStore.Load(knowledge.ChunkID(record.ID)); err == nil && ok {
			record.ContentHash = chunk.ContentHash
			record.Body = chunk.Body.Raw
			record.TokenEstimate = chunk.TokenEstimate
			kept = append(kept, record)
		}
	}
	summary.Chunks = kept
	return summary
}
