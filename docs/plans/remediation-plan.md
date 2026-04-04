 Chat Context Enrichment Pipeline — Gap Remediation     

 Context

 The gap analysis of the chat context enrichment pipeline
 (named/euclo/runtime/pretask/, named/euclo/interaction/modes/chat.go)
 revealed three categories of issues:

 1. Security model bypass — Evidence items carry no TrustClass, no
 PolicySnapshot is captured before retrieval, FileResolver skips
 permission checks, and InsertionDecision is never evaluated before
 proposing content to the user.
 2. Phase 4 incomplete — loadSessionPinsFromMemory /
 saveSessionPinsToMemory in chat.go:324-335 are stubs; knowledge items
 from the pipeline are never injected into SemanticInputBundle in
 agent.go.
 3. Test gaps — anchor_test.go doesn't exist, hypothetical_test.go has
 no assertions, merger_test.go doesn't exist, retrieval tests are stubs
  with no meaningful coverage. There is also a pre-existing compile
 error: context_proposal_test.go assigns *MockFileResolver to
 ContextProposalPhase.FileResolver which is typed *pretask.FileResolver
  — this blocks go test ./....

 Note: the confirmed-files-loading gap is already implemented in
 runtime_impl.go:96-101 (reads context.confirmed_files from task
 context → calls DrillDown).

 ---
 Step 0 — Fix pre-existing compile error (unblocks all tests)

 File: named/euclo/interaction/modes/chat.go

 - Define a narrow interface for file resolution:
 type FileResolverInterface interface {
     Resolve(selections []string, text string) pretask.ResolvedFiles
 }
 - Change ContextProposalPhase.FileResolver from *pretask.FileResolver
 → FileResolverInterface
 - Change ChatMode() parameter fileResolver *pretask.FileResolver →
 fileResolver FileResolverInterface
 - The call site in agent.go:1608 passes a *pretask.FileResolver — this
  is fine since *pretask.FileResolver satisfies the interface (pointer
 receiver on Resolve).
 - context_proposal_test.go:29 MockFileResolver already implements the
 interface correctly.

 ---
 Step A — Security: TrustClass on evidence items

 File: named/euclo/runtime/pretask/types.go

 Add TrustClass string to CodeEvidenceItem (after Source, ~line 33) and
  KnowledgeEvidenceItem (after Source, ~line 44). Use string — avoids
 adding a framework/core import to this leaf package; the constants are
  just the string values from core.TrustClass.

 File: named/euclo/runtime/pretask/retrieval.go

 In IndexRetriever.Retrieve(): items from anchors.FilePaths (directly
 user-specified) → TrustClass: "workspace-trusted". Items from
 symbol/dependency expansion → TrustClass: "builtin-trusted".

 In ArchaeoRetriever.RetrieveTopic() and RetrieveExpanded(): all
 returned KnowledgeEvidenceItem → TrustClass: "builtin-trusted".

 ---
 Step B — Security: PolicySnapshot in Pipeline

 File: named/euclo/runtime/pretask/types.go

 Add to EnrichedContextBundle:
 PolicySnapshot *core.PolicySnapshot
 This requires import "github.com/lexcodex/relurpify/framework/core" in
  types.go.

 File: named/euclo/runtime/pretask/pipeline.go

 Add to PipelineEnv:
 PolicySnapshotProvider func() *core.PolicySnapshot

 In Pipeline.Run() (line ~67), just before Stage 1:
 if p.env.PolicySnapshotProvider != nil {
     bundle.PolicySnapshot = p.env.PolicySnapshotProvider()
 }

 File: named/euclo/agent.go (~line 258, where pretask.PipelineEnv is
 constructed)

 env := pretask.PipelineEnv{
     IndexManager:          a.WorkspaceEnv.IndexManager,
     Model:                 a.WorkspaceEnv.Model,
     Embedder:              a.WorkspaceEnv.Embedder,
     PatternStore:          a.WorkspaceEnv.PatternStore,
     KnowledgeStore:        a.WorkspaceEnv.KnowledgeStore,
     PolicySnapshotProvider: func() *core.PolicySnapshot {
         if a.WorkspaceEnv.Registry == nil {
             return nil
         }
         return a.WorkspaceEnv.Registry.CapturePolicySnapshot()
     },
 }

 ---
 Step C — Security: PermissionManager in FileResolver

 File: named/euclo/runtime/pretask/resolver.go

 Add to FileResolver struct:
 CheckFileAccess func(path string) error

 In Resolve() (after existing workspace boundary check, ~line 40):
 if r.CheckFileAccess != nil {
     if err := r.CheckFileAccess(resolved); err != nil {
         result.Skipped = append(result.Skipped, resolved)
         continue
     }
 }

 File: named/euclo/agent.go (~line 1580, where fileResolver is built)

 fileResolver := &pretask.FileResolver{Workspace: workspacePath}
 if pm := a.WorkspaceEnv.PermissionManager; pm != nil {
     fileResolver.CheckFileAccess = func(path string) error {
         return pm.CheckFileAccess(context.Background(),
 a.Config.AgentID, core.FileSystemActionRead, path)
     }
 }

 Note: core.FileSystemActionRead — verify the exact constant name in
 framework/core.

 ---
 Step D — Security: InsertionAction on proposal content

 File: named/euclo/interaction/content.go

 Add InsertionAction string to ContextFileEntry (~line 177) and
 ContextKnowledgeEntry (~line 185).

 File: named/euclo/interaction/modes/chat.go

 Add helper:
 func trustClassToInsertionAction(tc string) string {
     switch tc {
     case "builtin-trusted", "workspace-trusted":
         return "direct"
     case "remote-approved":
         return "summarized"
     default:
         if tc == "" {
             return "direct" // unclassified internal content, default
 allow
         }
         return "metadata-only"
     }
 }

 In convertToContextProposalContent() (~line 337), when building
 ContextFileEntry and ContextKnowledgeEntry, set:
 InsertionAction: trustClassToInsertionAction(item.TrustClass),

 ---
 Step E — Phase 4: HybridMemory session pin persistence

 File: named/euclo/interaction/modes/chat.go

 Add import "github.com/lexcodex/relurpify/framework/memory" if not
 present.

 Add Memory memory.MemoryStore to ContextProposalPhase struct.

 Replace stubs at lines 324-335:
 func loadSessionPinsFromMemory(ctx context.Context, mem
 memory.MemoryStore) []string {
     if mem == nil {
         return nil
     }
     record, err := mem.Recall(ctx, "context.session_pins",
 memory.MemoryScopeProject)
     if err != nil {
         return nil
     }
     if pins, ok := record.Value.([]string); ok {
         return pins
     }
     return nil
 }

 func saveSessionPinsToMemory(ctx context.Context, mem
 memory.MemoryStore, pins []string) {
     if mem == nil || len(pins) == 0 {
         return
     }
     _ = mem.Remember(ctx, "context.session_pins", pins,
 memory.MemoryScopeProject)
 }

 In Execute(): before getSessionPins(mc.State), if p.Memory != nil and
 no "context.pinned_files" in state, load from memory and seed state.
 After each updatedPins computation (all 4 response branches ~lines
 117, 194, 226, 248), call saveSessionPinsToMemory(ctx, p.Memory,
 updatedPins).

 ChatMode(): add mem memory.MemoryStore parameter, pass to
 ContextProposalPhase{..., Memory: mem}.

 ChatModeLegacy() (~line 469): pass nil for memory: ChatMode(emitter,
 resolver, nil, nil, true, nil).

 ChatModeWithContext() (~line 457): pass workspaceEnv.Memory.

 File: named/euclo/agent.go (~line 1608)

 return modes.ChatMode(emitter, resolver, pipeline, fileResolver,
 showConfirmationFrame, a.WorkspaceEnv.Memory)

 ---
 Step F — Phase 4: Knowledge items → SemanticInputBundle

 File: named/euclo/agent.go

 Add helper (after semanticInputBundle function):
 func enrichBundleWithContextKnowledge(bundle
 eucloruntime.SemanticInputBundle, state *core.Context)
 eucloruntime.SemanticInputBundle {
     raw, ok := state.Get("context.knowledge_items")
     if !ok {
         return bundle
     }
     items, ok := raw.([]pretask.KnowledgeEvidenceItem)
     if !ok || len(items) == 0 {
         return bundle
     }
     for _, item := range items {
         finding := eucloruntime.SemanticFindingSummary{
             RefID:       item.RefID,
             Kind:        "context_retrieved_" + string(item.Kind),
             Status:      "retrieved",
             Title:       item.Title,
             Summary:     item.Summary,
             RelatedRefs: append([]string(nil), item.RelatedRefs...),
         }
         bundle.PatternFindings = append(bundle.PatternFindings,
 finding)
     }
     return bundle
 }

 In semanticInputBundle(), wrap both return paths:
 - Line ~537 (non-planning cached path): return
 enrichBundleWithContextKnowledge(typed, state)
 - Line ~538 (non-planning empty path): return
 enrichBundleWithContextKnowledge(eucloruntime.SemanticInputBundle{},
 state)
 - Line ~575 (planning/debug/review path after
 EnrichSemanticInputBundle): return enrichBundleWithContextKnowledge(eu
 cloarchaeomem.EnrichSemanticInputBundle(...), state)

 Verify pretask is already imported in agent.go — it is (used for
 ContextPipeline *pretask.Pipeline).

 ---
 Step G — Tests: anchor_test.go (new file)

 File: named/euclo/runtime/pretask/anchor_test.go

 Tests use dummyIndexQuerier pattern. Create a stubIndexQuerier that
 returns a pre-configured set of matching symbol names.

 - TestAnchorExtract_CurrentTurnFilesIncluded — input with
 CurrentTurnFiles: ["a.go"], verify a.go in AnchorSet.FilePaths
 - TestAnchorExtract_SessionPinsIncluded — SessionPins: ["pinned.go"],
 verify in FilePaths
 - TestAnchorExtract_AtMentionExtraction — query "look at @cmd/main.go
 for details", verify cmd/main.go in FilePaths
 - TestAnchorExtract_CamelCaseConfirmedByIndex — query "fix MyHandler
 bug", stub index returns a node for MyHandler, verify MyHandler in
 SymbolNames
 - TestAnchorExtract_CamelCaseFilteredByIndex — stub returns nil for
 every symbol, verify SymbolNames is empty
 - TestAnchorExtract_EmptyInput — zero-value AnchorExtractInput, verify
  nothing panics and AnchorSet is empty

 ---
 Step H — Tests: hypothetical_test.go (fill in)

 File: named/euclo/runtime/pretask/hypothetical_test.go

 Existing test (TestHypotheticalGenerator_Generate with nil model) —
 rename to TestHypotheticalGenerator_NilModelReturnsUngrounded and
 keep.

 Add:
 - TestHypotheticalGenerator_StubModelReturnsGrounded — stub LLM
 implementing core.LanguageModel returns *core.LLMResponse{Content:
 []core.ContentBlock{{Type: "text", Text: "MyHandler fileSystem"}}}. No
  embedder needed (nil embedder path). Verify sketch.Grounded == false
 (no embedder), sketch.Text != "".
 - TestHypotheticalGenerator_NilEmbedderSetsGroundedFalse — same stub
 model, nil embedder, verify Grounded == false but Text is populated
 (generation succeeded, embedding skipped).

 ---
 Step I — Tests: merger_test.go (new file)

 File: named/euclo/runtime/pretask/merger_test.go

 - TestResultMerger_AnchoredFilesAlwaysIncluded — token budget = 1, one
  anchored file → appears in AnchoredFiles, expanded does not (budget
 exhausted)
 - TestResultMerger_DeduplicatesByPath — same path in stage1.IndexFiles
  and expanded.ExpandedFiles → appears once in output
 - TestResultMerger_KnowledgeDeduplicatedByRefID — same RefID in
 KnowledgeTopic and KnowledgeExpanded → appears once
 - TestResultMerger_TokenBudgetRespected — large token budget with many
  items → total token count stays within budget (use PipelineTrace to
 verify)
 - TestResultMerger_TracePopulated — verify PipelineTrace fields (e.g.,
  AnchorsExtracted) are non-zero after merge

 ---
 Step J — Tests: retrieval_test.go (enhance)

 File: named/euclo/runtime/pretask/retrieval_test.go

 After Step A assigns TrustClass in retrieval:

 - TestIndexRetriever_AssignsTrustClass — items from anchors.FilePaths
 have TrustClass == "workspace-trusted", expansion items have
 "builtin-trusted"
 - TestArchaeoRetriever_EmptyWorkflowIDSkipsRetrieval —
 ArchaeoRetrieverConfig{WorkflowID: ""} → RetrieveTopic() returns empty
  slice without error
 - TestArchaeoRetriever_KnowledgeItemsTrustClass — returned knowledge
 items have TrustClass == "builtin-trusted"

 ---
 Verification

 go build ./...                                    # must be clean
 go test ./named/euclo/runtime/pretask/...         # all new tests pass
 go test ./named/euclo/interaction/...             # compile error
 fixed, all tests pass
 go test ./...                                     # complete suite
 passes

 ---
 Critical files modified

 ┌───────────────────────────────────┬─────────────────────────────┐
 │               File                │           Changes           │
 ├───────────────────────────────────┼─────────────────────────────┤
 │                                   │ TrustClass on CodeEvidenceI │
 │ named/euclo/runtime/pretask/types │ tem/KnowledgeEvidenceItem;  │
 │ .go                               │ PolicySnapshot on           │
 │                                   │ EnrichedContextBundle       │
 ├───────────────────────────────────┼─────────────────────────────┤
 │ named/euclo/runtime/pretask/retri │ Assign TrustClass on        │
 │ eval.go                           │ returned items              │
 ├───────────────────────────────────┼─────────────────────────────┤
 │ named/euclo/runtime/pretask/pipel │ PolicySnapshotProvider in   │
 │ ine.go                            │ PipelineEnv; capture in     │
 │                                   │ Run()                       │
 ├───────────────────────────────────┼─────────────────────────────┤
 │ named/euclo/runtime/pretask/resol │ CheckFileAccess func field; │
 │ ver.go                            │  skip files that fail check │
 ├───────────────────────────────────┼─────────────────────────────┤
 │ named/euclo/interaction/content.g │ InsertionAction on          │
 │ o                                 │ ContextFileEntry/ContextKno │
 │                                   │ wledgeEntry                 │
 ├───────────────────────────────────┼─────────────────────────────┤
 │                                   │ FileResolverInterface;      │
 │ named/euclo/interaction/modes/cha │ Memory field; implement     │
 │ t.go                              │ HybridMemory stubs;         │
 │                                   │ InsertionAction wiring      │
 ├───────────────────────────────────┼─────────────────────────────┤
 │                                   │ PolicySnapshotProvider      │
 │ named/euclo/agent.go              │ closure; CheckFileAccess    │
 │                                   │ closure; memory threading;  │
 │                                   │ knowledge injection         │
 ├───────────────────────────────────┼─────────────────────────────┤
 │ named/euclo/runtime/pretask/ancho │ New — 6 tests               │
 │ r_test.go                         │                             │
 ├───────────────────────────────────┼─────────────────────────────┤
 │ named/euclo/runtime/pretask/hypot │ 2 additional tests          │
 │ hetical_test.go                   │                             │
 ├───────────────────────────────────┼─────────────────────────────┤
 │ named/euclo/runtime/pretask/merge │ New — 5 tests               │
 │ r_test.go                         │                             │
 ├───────────────────────────────────┼─────────────────────────────┤
 │ named/euclo/runtime/pretask/retri │ 3 additional tests          │
 │ eval_test.go                      │                             │
 └───────────────────────────────────┴─────────────────────────────┘