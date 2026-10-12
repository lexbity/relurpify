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
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
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

	// go_native tool constructor registrations: the harness composes the
	// registry from the materialized shipped manifests, exactly as the
	// production runtime does (app/relurpish/runtime/tool_activation.go).
	// Without these, strict-mode build skips every go_native manifest and the
	// loud load (D2) correctly rejects the recipes that declare them.
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/fs"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/git"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/langgo"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/langjs"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/langpython"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/langrust"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/lsp"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/search"
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
	// SelectionPosture overrides the route-selection posture for this run:
	// "deterministic" (default) never consults the tier-2 model; a run that
	// specifically tests model-assisted selection sets "model_assisted".
	SelectionPosture string
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

	// Settle the retriever's event subscription: seed grounding publishes
	// chunk events on the bus, and the retriever's consumer goroutine
	// invalidates its snapshot generation asynchronously. The intake compile
	// is the FIRST retrieval of the run — without a deterministic settle it
	// races the consumer and misses the seeds. Poll until a probe retrieval
	// surfaces the last-seeded body (bounded), so the intake delivery the
	// suite asserts is the run's real behavior, not a race outcome.
	if len(cfg.SeedChunks) > 0 {
		deadline := time.Now().Add(2 * time.Second)
		for {
			result, err := kn.Retriever.Retrieve(ctx, retrieval.RetrievalQuery{
				Text: cfg.SeedChunks[len(cfg.SeedChunks)-1].Body,
			})
			settled := err == nil && result != nil && len(result.Ranked) > 0
			if settled || time.Now().After(deadline) {
				break
			}
			time.Sleep(time.Millisecond)
		}
	}

	model := testhelper.NewSequencedModel(cfg.Turns...)
	maxIterations := cfg.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 8
	}
	deps := &paradigm.Deps{
		Config: &execution.Config{
			Name:             "prep",
			Model:            "scripted",
			MaxIterations:    maxIterations,
			Workspace:        ws,
			SelectionPosture: cfg.SelectionPosture,
			Telemetry:        sink,
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
	if err := agent.Initialize(&execution.Config{Workspace: ws, SelectionPosture: cfg.SelectionPosture}); err != nil {
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
	// The shipped tool manifests materialize for every workspace: the
	// canonical recipes declare file tools (file_read/grep/file_write), and
	// the built-in default execution recipe (registered programmatically for
	// every run) declares the same file toolset. The registry admits only
	// manifest-declared tools, so without the manifests the loud load (D2)
	// correctly rejects both the recipes and the default fallback's scope.
	if err := copyEmbeddedTree("workspace/tools", filepath.Join(ws, "relurpify_cfg", "tools")); err != nil {
		return fmt.Errorf("prep: tool manifests: %w", err)
	}
	if !canonical {
		return nil
	}
	return copyEmbeddedTree("workspace/euclo", filepath.Join(ws, "relurpify_cfg", "euclo"))
}

// copyEmbeddedTree materializes an embedded template subtree into the
// workspace, preserving relative paths.
func copyEmbeddedTree(embedRoot, destRoot string) error {
	embed, err := iofs.Sub(embedfs.DefaultFS(), embedRoot)
	if err != nil {
		return err
	}
	return iofs.WalkDir(embed, ".", func(path string, d iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(destRoot, filepath.FromSlash(path))
		if d.IsDir() {
			return capfs.MkdirAllSecure(target)
		}
		content, err := iofs.ReadFile(embed, path)
		if err != nil {
			return err
		}
		return capfs.WriteFileSecure(target, content)
	})
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
