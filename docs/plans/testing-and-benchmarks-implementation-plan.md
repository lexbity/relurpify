Implementation Plan: testfu + Euclo E2E Testing

  ---
  Phase 0 — Wiring Fixes (Prerequisites)

  These are correctness bugs that will cause false failures or silent skips in all E2E runs. They must be resolved first.

  0.1 — Tool call swallowed at iteration boundary (agents/react/react_think_node.go)

  What's broken: In normalizeDecision, when ParseToolCallsFromText finds nothing, parseDecision fails, and repairDecision also fails (line 127), the
  function returns Complete: true discarding any embedded tool call. The repair LLM call can fail under token pressure or when the model returns garbled
  output.

  Fix: Add a final fallback before returning Complete: true — if resp.Text contains any tool-like JSON structure, set Complete: false and let the observe
  node handle it rather than silently terminating. Also add a unit test that exercises the repair-fail path with an embedded tool call in prose and asserts
  the call is not silently dropped.

  Files: agents/react/react_think_node.go, agents/react/react_test.go

  0.2 — IndexManager not passed to ReActAgent (app/relurpish/runtime/bootstrap.go)

  What's broken: agents.WithIndexManager(indexManager) is called in bootstrap but tracing confirms the pointer never reaches the ReActAgent's field — it's
  wired to the environment but the react agent's InitializeEnvironment path doesn't pick it up. Progressive loading becomes a no-op at runtime.

  Fix: Verify the full chain from bootstrap.go:WithIndexManager → agentenv.AgentEnvironment.IndexManager → ReActAgent.IndexManager. Add an integration
  assertion in bootstrap_test.go that a bootstrapped react agent has a non-nil IndexManager after initialization.

  Files: app/relurpish/runtime/bootstrap.go, agents/react/react.go, agents/react/react_test.go

  0.3 — ModeDocument write restriction is prompt-only (agents/react/react.go)

  What's broken: docs mode sets react.phase to contextmgrPhaseEdit (line 719-721) which influences which tools are offered, but file_write/file_create are
  not actually gated — the tool availability filter doesn't exclude write tools for docs mode.

  Fix: Add a toolAllowedByExecutionContext rule that denies file_write, file_create, file_delete when a.Mode == "docs". Add a unit test: docs-mode agent
  presented with a write task should not invoke a write tool.

  Files: agents/react/react.go, agents/react/react_test.go

  ---
  Phase 1 — testfu Enhancements

  The agent core is solid. These additions close the gap between "run one suite" and "orchestrate a testing session."

  1.1 — Multi-suite action with agent-scoped suite discovery

  What's needed: executeRequest only dispatches to a single suite. Add actionRunAgent — given agent_name, it globs all matching *.testsuite.yaml files,
  filters by optional lane/tags, and runs them sequentially. Return a map[string]*SuiteReport keyed by suite name.

  Contract addition to parseRequest:
  agent_name: react          # runs all react.*.testsuite.yaml
  tags: smoke                # optional tag filter applied via FilterSuiteCasesByTags
  lane: pr-smoke             # passed to RunOptions.Lane

  New state keys: testfu.agent_suites_report (map of suite name → SuiteReport), testfu.total_passed, testfu.total_failed, testfu.total_skipped.

  Files: named/testfu/agent.go, named/testfu/request.go, named/testfu/report.go, named/testfu/options.go

  1.2 — New tool: testfu:run_agent_suites

  Add a fourth execution tool that wraps actionRunAgent. Parameters: agent_name (required), tags (optional), lane (optional), timeout (optional).

  Files: named/testfu/tools.go

  1.3 — Timeout budgeting

  When running multiple suites, the total execution time can overrun the outer task timeout. Add BudgetedTimeout logic: distribute the remaining time budget
   across pending suites, with a per-suite minimum floor (e.g. 30s). If budget is exhausted, mark remaining suites as skipped rather than crashing.

  Files: named/testfu/agent.go, new helper in named/testfu/runner/budget.go

  1.4 — Expand agent_test.go

  Add tests for:
  - actionRunAgent dispatches to all matching suites (fakeRunner called N times)
  - BudgetedTimeout skips suites when budget is exhausted
  - tags filter applied before running
  - failedCaseNames handles map[string]*SuiteReport result type

  Files: named/testfu/agent_test.go

  ---
  Phase 2 — testfu YAML Suites

  2.1 — testsuite/agenttests/testfu.smoke.testsuite.yaml

  Agent: testfu. Tier: smoke.

  ┌───────────────────────┬───────────────────────────────────┬───────────────────────────────────────┐
  │         Case          │           What it does            │                Expect                 │
  ├───────────────────────┼───────────────────────────────────┼───────────────────────────────────────┤
  │ list_suites           │ action: list_suites               │ output_contains: react, planner       │
  ├───────────────────────┼───────────────────────────────────┼───────────────────────────────────────┤
  │ run_react_smoke       │ runs react.testsuite.yaml         │ testfu.passed=true, no failures       │
  ├───────────────────────┼───────────────────────────────────┼───────────────────────────────────────┤
  │ run_single_case       │ targets react / edit_fixture_file │ testfu.passed=true, report has 1 case │
  ├───────────────────────┼───────────────────────────────────┼───────────────────────────────────────┤
  │ run_nonexistent_suite │ bad path                          │ must_succeed: false                   │
  ├───────────────────────┼───────────────────────────────────┼───────────────────────────────────────┤
  │ run_with_tag_filter   │ react suite + tag: smoke          │ only smoke-tagged cases run           │
  └───────────────────────┴───────────────────────────────────┴───────────────────────────────────────┘

  2.2 — testsuite/agenttests/testfu.paradigm.testsuite.yaml

  Agent: testfu. Tier: stable.

  ┌─────────────────────┬──────────────────┬─────────────────────────────────┬──────────────────────┐
  │        Case         │ Agent under test │             Suites              │        Expect        │
  ├─────────────────────┼──────────────────┼─────────────────────────────────┼──────────────────────┤
  │ paradigm_react      │ react            │ react.testsuite.yaml            │ all cases pass       │
  ├─────────────────────┼──────────────────┼─────────────────────────────────┼──────────────────────┤
  │ paradigm_planner    │ planner          │ planner.testsuite.yaml          │ all cases pass       │
  ├─────────────────────┼──────────────────┼─────────────────────────────────┼──────────────────────┤
  │ paradigm_htn        │ htn              │ htn.testsuite.yaml              │ all cases pass       │
  ├─────────────────────┼──────────────────┼─────────────────────────────────┼──────────────────────┤
  │ paradigm_rewoo      │ rewoo            │ rewoo.testsuite.yaml            │ all cases pass       │
  ├─────────────────────┼──────────────────┼─────────────────────────────────┼──────────────────────┤
  │ paradigm_reflection │ reflection       │ reflection.testsuite.yaml       │ all cases pass       │
  ├─────────────────────┼──────────────────┼─────────────────────────────────┼──────────────────────┤
  │ paradigm_all_smoke  │ (multi)          │ all paradigm suites, tag: smoke │ all smoke cases pass │
  └─────────────────────┴──────────────────┴─────────────────────────────────┴──────────────────────┘

  2.3 — testsuite/agenttests/testfu.euclo.testsuite.yaml

  Agent: testfu. Tier: stable. Timeout: 300s (inner euclo cases are 90s each).

  ┌────────────────────┬─────────────────────────────────────────────────┬─────────────────────────────────────┐
  │        Case        │                   Euclo suite                   │               Expect                │
  ├────────────────────┼─────────────────────────────────────────────────┼─────────────────────────────────────┤
  │ euclo_code_smoke   │ euclo.code.testsuite.yaml, tag: coverage-matrix │ core cases pass                     │
  ├────────────────────┼─────────────────────────────────────────────────┼─────────────────────────────────────┤
  │ euclo_debug_smoke  │ euclo.debug.testsuite.yaml                      │ passes                              │
  ├────────────────────┼─────────────────────────────────────────────────┼─────────────────────────────────────┤
  │ euclo_review_smoke │ euclo.review.testsuite.yaml                     │ passes                              │
  ├────────────────────┼─────────────────────────────────────────────────┼─────────────────────────────────────┤
  │ euclo_chat_smoke   │ euclo.chat.testsuite.yaml                       │ passes (once chat suite is written) │
  └────────────────────┴─────────────────────────────────────────────────┴─────────────────────────────────────┘

  ---
  Phase 3 — Paradigm Agent Suite Expansion

  The goal is meaningful paradigm coverage — not just "does it start" but "does the paradigm behave as specified."

  3.1 — react.testsuite.yaml — expand from 2 → 6 cases

  ┌────────────────────────────┬────────────────────────────────────────────────────────────┬───────────────────────────────────────────────────────────┐
  │          New Case          │                           Tests                            │                      Key assertions                       │
  ├────────────────────────────┼────────────────────────────────────────────────────────────┼───────────────────────────────────────────────────────────┤
  │ multi_step_read_then_write │ Read a file, decide on edit, write                         │ tool_calls_must_include: [file_read, file_write], ordered │
  ├────────────────────────────┼────────────────────────────────────────────────────────────┼───────────────────────────────────────────────────────────┤
  │ tool_error_recovery        │ First file_write to a forbidden path, then to correct path │ Success, retried write with corrected path                │
  ├────────────────────────────┼────────────────────────────────────────────────────────────┼───────────────────────────────────────────────────────────┤
  │ read_only_analysis         │ Analyse two files, no edits                                │ no_file_changes: true, output contains analysis           │
  ├────────────────────────────┼────────────────────────────────────────────────────────────┼───────────────────────────────────────────────────────────┤
  │ negative_unknown_tool      │ Prompt asks for a non-existent tool                        │ Success (agent recovers), no crash                        │
  └────────────────────────────┴────────────────────────────────────────────────────────────┴───────────────────────────────────────────────────────────┘

  3.2 — planner.testsuite.yaml — expand from 1 → 5 cases

  ┌────────────────────┬────────────────────────────────────────────┐
  │      New Case      │                   Tests                    │
  ├────────────────────┼────────────────────────────────────────────┤
  │ plan_then_edit     │ Plan emitted, then file edited per plan    │
  ├────────────────────┼────────────────────────────────────────────┤
  │ multi_file_plan    │ Plan spans 2 files, both edited            │
  ├────────────────────┼────────────────────────────────────────────┤
  │ plan_step_recovery │ First step fails (bad path), plan recovers │
  ├────────────────────┼────────────────────────────────────────────┤
  │ read_only_plan     │ Analysis plan, no writes, summary returned │
  └────────────────────┴────────────────────────────────────────────┘

  3.3 — htn.testsuite.yaml — expand to 5 cases

  Add:
  - htn_primitive_fallback — top-level task decomposes, one primitive method fails, fallback method used
  - htn_resume_from_checkpoint — seed an in-progress workflow, verify HTN resumes from correct step
  - htn_multi_method_selection — two valid methods for a task, verify correct one selected based on context

  3.4 — rewoo.testsuite.yaml — assess then expand

  Read current cases, add:
  - rewoo_reason_before_act — verify plan is emitted before any tool call
  - rewoo_observation_integration — observation from tool result influences next reasoning step

  3.5 — reflection.testsuite.yaml — assess then expand

  Add:
  - reflection_self_corrects — initial output is wrong, reflection step identifies and corrects it
  - reflection_no_regression — good initial output, reflection does not change it

  ---
  Phase 4 — Euclo Coverage Gaps

  4.1 — testsuite/agenttests/euclo.chat.testsuite.yaml (new)

  Mode: chat. The most-used euclo mode has zero suite coverage.

  ┌──────────────────────────┬──────────────────────┬───────────────────────────────┬──────────────────────────────────────────────────────────────┐
  │           Case           │      Capability      │             Setup             │                            Expect                            │
  ├──────────────────────────┼──────────────────────┼───────────────────────────────┼──────────────────────────────────────────────────────────────┤
  │ chat_ask_analysis        │ euclo:chat.ask       │ File with a Go struct         │ Explains what the struct does, phases_executed: [intent]     │
  ├──────────────────────────┼──────────────────────┼───────────────────────────────┼──────────────────────────────────────────────────────────────┤
  │ chat_inspect_interface   │ euclo:chat.inspect   │ Interface + 2 implementations │ Describes differences, no file changes                       │
  ├──────────────────────────┼──────────────────────┼───────────────────────────────┼──────────────────────────────────────────────────────────────┤
  │ chat_implement_small     │ euclo:chat.implement │ Function stub + docstring     │ Stub filled in, phases_executed: [intent, propose]           │
  ├──────────────────────────┼──────────────────────┼───────────────────────────────┼──────────────────────────────────────────────────────────────┤
  │ chat_ask_no_context      │ euclo:chat.ask       │ No files provided             │ Answers from general knowledge, must_succeed: true           │
  ├──────────────────────────┼──────────────────────┼───────────────────────────────┼──────────────────────────────────────────────────────────────┤
  │ chat_implement_with_test │ euclo:chat.implement │ Stub + failing test           │ Implementation passes test, files_changed includes stub file │
  └──────────────────────────┴──────────────────────┴───────────────────────────────┴──────────────────────────────────────────────────────────────┘

  4.2 — testsuite/agenttests/euclo.archaeology.testsuite.yaml (new or major expansion)

  Mode: archaeology. Covers the explore → compile-plan → implement-plan capability chain.

  ┌───────────────────────────────┬──────────────────────────────────┬───────────────────────────┬──────────────────────────────────────────────────────┐
  │             Case              │            Capability            │           Setup           │                        Expect                        │
  ├───────────────────────────────┼──────────────────────────────────┼───────────────────────────┼──────────────────────────────────────────────────────┤
  │ explore_codebase_pattern      │ euclo:archaeology.explore        │ 3-file Go package         │ Exploration record produced, artifacts_produced:     │
  │                               │                                  │                           │ [exploration]                                        │
  ├───────────────────────────────┼──────────────────────────────────┼───────────────────────────┼──────────────────────────────────────────────────────┤
  │ compile_plan_from_exploration │ euclo:archaeology.compile-plan   │ Seed exploration workflow │ Plan artifact produced, artifacts_produced:          │
  │                               │                                  │                           │ [compiled_plan]                                      │
  ├───────────────────────────────┼──────────────────────────────────┼───────────────────────────┼──────────────────────────────────────────────────────┤
  │ implement_from_plan           │ euclo:archaeology.implement-plan │ Seed compiled plan        │ Files changed per plan, plan marked complete         │
  ├───────────────────────────────┼──────────────────────────────────┼───────────────────────────┼──────────────────────────────────────────────────────┤
  │ archaeology_resume            │ full chain                       │ Seed partial workflow     │ Resumes from last checkpoint, completes remaining    │
  │                               │                                  │ state                     │ steps                                                │
  ├───────────────────────────────┼──────────────────────────────────┼───────────────────────────┼──────────────────────────────────────────────────────┤
  │ archaeology_recovery          │ implement-plan                   │ Seed plan with one broken │ Recovery strategy fires, fallback used               │
  │                               │                                  │  step                     │                                                      │
  └───────────────────────────────┴──────────────────────────────────┴───────────────────────────┴──────────────────────────────────────────────────────┘

  4.3 — Artifact-mediated cases (add to existing suites)                                                                                                    
  
  Add one case per relevant euclo suite that provides a messy context bundle rather than a clean prompt. Modelled on the benchmark document's               
  "artifact-mediated engineering benchmark" concept.
                                                                                                                                                            
  - euclo.code: case code_from_messy_context — setup includes a conflicting bug report + partial spec + existing code with a subtle bug. Prompt is terse    
  ("fix this as discussed"). Expect euclo recovers the correct intent and makes the right change.
  - euclo.debug: case debug_from_incident_log — setup includes simulated log output + stack trace inline in prompt. No clean reproduction steps. Expect     
  reproduce_localize_patch profile selected.                                                                                                                
  - euclo.planning: case planning_from_partial_adr — setup includes an incomplete ADR (Architecture Decision Record) with TODOs. Expect euclo emits a plan
  that respects the implied constraints.                                                                                                                    
                  
  4.4 — Mode auto-classification under ambiguity (add to euclo.classification.testsuite.yaml)                                                               
                  
  Verify that euclo selects the right mode when the prompt doesn't include explicit mode hints:                                                             
                  
  ┌─────────────────────────────────┬──────────────────────────────────────────────────────┬───────────────┐                                                
  │              Case               │                     Prompt style                     │ Expected mode │
  ├─────────────────────────────────┼──────────────────────────────────────────────────────┼───────────────┤                                                
  │ classify_debug_from_error       │ "Getting panic: nil pointer dereference at..."       │ debug         │
  ├─────────────────────────────────┼──────────────────────────────────────────────────────┼───────────────┤
  │ classify_review_from_pr_request │ "Please review my changes to pkg/auth"               │ review        │                                                
  ├─────────────────────────────────┼──────────────────────────────────────────────────────┼───────────────┤                                                
  │ classify_planning_from_scope    │ "We need to migrate from X to Y, where do we start?" │ planning      │                                                
  ├─────────────────────────────────┼──────────────────────────────────────────────────────┼───────────────┤                                                
  │ classify_code_from_imperative   │ "Add a method that does X"                           │ code          │
  └─────────────────────────────────┴──────────────────────────────────────────────────────┴───────────────┘                                                
                  
  These exist conceptually in euclo.classification.testsuite.yaml — verify coverage and fill gaps.                                                          
                  
  4.5 — Capability contract enforcement cases                                                                                                               
                  
  Add cases specifically asserting that pre/post-execution capability contracts fire correctly.                                                             
                  
  - Add to euclo.capability_interactions.testsuite.yaml:                                                                                                    
    - contract_pre_execution_guard_fires — task requiring a write cap that isn't in the admitted set → expect denied result, not crash
    - contract_lazy_semantic_acquisition — capability acquired lazily mid-execution → euclo.capability_contract_runtime.LazySemanticAcquisitionTriggered =  
  true in state                                                                                                                                             
                                                                                                                                                            
  ---                                                                                                                                                       
  Phase 5 — Benchmark-Aligned Advanced Cases
                                            
  These implement the hierarchical capability levels from the benchmark document. They're the most expensive to write but provide the strongest signal about
   real-world capability.                                                                                                                                   
  
  5.1 — Intent fidelity cases                                                                                                                               
                  
  One new suite: euclo.intent_fidelity.testsuite.yaml.                                                                                                      
                  
  Each case gives euclo an intentionally incomplete or conflicting context bundle and measures whether it recovers the correct intent. No explicit mode     
  hint. No clean task description.
                                                                                                                                                            
  ┌────────────────────────────────┬────────────────────────────────────────────────┬─────────────────────────────┬─────────────────────────────────────┐   
  │              Case              │                Bundle contents                 │     Ground truth intent     │            Pass criteria            │
  ├────────────────────────────────┼────────────────────────────────────────────────┼─────────────────────────────┼─────────────────────────────────────┤   
  │ recover_intent_from_conflict   │ Bug report says "add X"; comment in code says  │ Implement X behind a        │ File changed with flag pattern      │
  │                                │ "never add X"; ADR says "X is planned for v2"  │ feature flag                │                                     │   
  ├────────────────────────────────┼────────────────────────────────────────────────┼─────────────────────────────┼─────────────────────────────────────┤   
  │ recover_from_symptom_report    │ Support ticket "the button doesn't work"; no   │ Fix the event handler       │ file_changed contains the handler   │
  │                                │ stack trace; 3 JS files provided               │ binding                     │ file                                │   
  ├────────────────────────────────┼────────────────────────────────────────────────┼─────────────────────────────┼─────────────────────────────────────┤
  │                                │ PR diff showing 2 rejected approaches + a      │ Follow the accepted         │ Output references accepted          │   
  │ infer_architectural_constraint │ third accepted one; new task is similar        │ pattern, not the rejected   │ approach; no use of rejected        │   
  │                                │                                                │ ones                        │ patterns                            │
  └────────────────────────────────┴────────────────────────────────────────────────┴─────────────────────────────┴─────────────────────────────────────┘   
                  
  5.2 — Hierarchical coverage matrix tagging                                                                                                                
  
  Add level: 1|2|3|4 tags to all euclo cases across all suites. This enables agenttest run --tag level:1 for smoke, --tag level:3 for intent-recovery       
  regression. Levels:
                                                                                                                                                            
  - Level 1 — local repair (fix a bug, change a value, pass a test)                                                                                         
  - Level 2 — scoped implementation (implement a function/method from spec, update tests/docs)
  - Level 3 — intent recovery (infer correct task from partial/conflicting artifacts)                                                                       
  - Level 4 — architectural alignment (choice must respect implied design direction)                                                                        
                                                                                                                                                            
  5.3 — Performance baseline cases                                                                                                                          
                                                                                                                                                            
  Add to euclo.performance_context.testsuite.yaml:                                                                                                          
  - performance_large_file_navigation — 800-line Go file, agent must find and patch one function. Baseline: no more than 2 context rescans.
  - performance_multi_file_coordination — 5 files, change must be consistent across all. Baseline: single exploration pass.                                 
                                                                                                                           
  Add corresponding entries to PerformanceBaseline in the runner so regressions surface automatically.                                                      
                                                                                                                                                            
  ---                                                                                                                                                       
  Dependency Graph                                                                                                                                          
                                                                                                                                                            
  Phase 0 (wiring fixes)
    └── Phase 3 (paradigm suite expansion)                                                                                                                  
          └── Phase 2.1 (testfu.smoke)
                └── Phase 1 (testfu enhancement)                                                                                                            
                      └── Phase 2.2 (testfu.paradigm)
                            └── Phase 2.3 (testfu.euclo)                                                                                                    
                  
  Phase 0 (wiring fixes)                                                                                                                                    
    └── Phase 4.1 (euclo.chat suite)
    └── Phase 4.2 (euclo.archaeology suite)                                                                                                                 
    └── Phase 4.3-4.5 (euclo additions)                                                                                                                     
          └── Phase 5 (benchmark-aligned cases)                                                                                                             
                                                                                                                                                            
  Phase 0 must go first. Phases 1-3 and Phases 4-5 are independent tracks that can be worked in parallel after Phase 0 completes. Phase 2.3 (testfu.euclo)  
  depends on both tracks being stable.                                                                                                                      
                                                                                                                                                            
  ---             
  Definition of Done (per phase)
                                                                                                                                                            
  - Phase 0: go test ./agents/react/... passes including new regression tests for each wiring fix
  - Phase 1: go test ./named/testfu/... passes; testfu:run_agent_suites tool registered and callable                                                        
  - Phase 2: all three testfu YAML suites runnable via agenttest run --agent testfu with Ollama live                                                        
  - Phase 3: each paradigm suite has ≥4 cases; agenttest run --agent react (etc.) passes all                                                                
  - Phase 4: euclo.chat and euclo.archaeology suites pass; artifact-mediated + classification gaps filled                                                   
  - Phase 5: intent fidelity suite exists with 3 cases; level tags applied across all euclo suites; performance baselines registered 
