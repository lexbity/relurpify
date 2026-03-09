Goal

Add a typed pipeline control flow to the framework without making it a one-off side system, then expose it through PipelineAgent and CodingAgent mode selection while preserving ReAct as another first-class runtime.

Design Principles

Keep framework multi-paradigm: ReAct and Pipeline are both control-flow runtimes.
Share common concerns: telemetry, persistence, context, capability policy, manifests.
Make pipeline stage boundaries typed and validated.
Keep migration incremental: add pipeline cleanly first, then move selected coding modes/workflows onto it.
Phase 1: Framework Contract Layer

Add a new package: framework/pipeline.
Define the core typed concepts:
StageSpec
StageResult
StageTransition
ContractMetadata
ValidationError
Define runtime interfaces.
type Stage interface
Name() string
Contract() ContractDescriptor
BuildPrompt(ctx *core.Context) (string, error)
Decode(resp *core.LLMResponse) (any, error)
Validate(output any) error
Apply(ctx *core.Context, output any) error
Define contract metadata.
input key
output key
schema version
retry policy
whether tools are allowed
Add a reusable typed envelope for persisted stage outputs.
raw prompt
raw response
decoded JSON
validation status
error text
retry count
Files

framework/pipeline/contracts.go
framework/pipeline/stage.go
framework/pipeline/types.go
framework/pipeline/errors.go
Unit Tests

contract metadata validation
stage decode failure handling
stage validate failure handling
stage apply success/failure behavior
Phase 2: Pipeline Runtime

Implement PipelineRunner.
executes ordered stages
calls prompt -> model -> decode -> validate -> apply
stops on hard validation/decode errors
supports bounded retries
Add transition support.
sequential by default
optional explicit branch hooks later
Integrate telemetry.
stage start
stage finish
stage decode error
stage validation error
Integrate checkpoint/persistence hooks.
persist each completed stage result
allow resume from next incomplete stage
Files

framework/pipeline/runner.go
framework/pipeline/telemetry.go
framework/pipeline/checkpoint.go
Unit Tests

happy-path multi-stage execution
decode failure stops pipeline
validation failure stops pipeline
retry policy works
resume from partially completed pipeline
telemetry emission per stage
Phase 3: Framework Persistence Extensions

Extend workflow persistence for stage-oriented records.
Add new records or extend artifact/event records with:
stage_name
contract_name
contract_version
decoded_output_json
validation_ok
retry_attempt
Add helper APIs:
SaveStageResult
ListStageResults
GetLatestStageResult
Keep this compatible with existing architect workflow persistence rather than replacing it immediately.
Files

framework/persistence/workflow_state_store.go
framework/persistence/sqlite_workflow_state_store.go
tests in framework/persistence/*_test.go
Unit Tests

persist/load stage result
persist validation failures
stage result ordering by attempt
resume lookup for latest valid stage
Phase 4: Pipeline Agent

Add PipelineAgent.
implements graph.Agent
owns a PipelineRunner
builds stage lists per task/mode
Keep BuildGraph support.
represent stage chain as graph nodes for inspection
one node per stage
Support mode/task-specific stage sets.
code modification
analysis
docs
debug
Add stage registry/factory so pipeline definitions are composable.
Files

agents/pipeline_agent.go
agents/pipeline_agent_test.go
optional agents/stages/registry.go
Unit Tests

PipelineAgent.Execute
PipelineAgent.BuildGraph
correct stage list chosen by task type/mode
persistence integration
failure propagation
Phase 5: Coding-Agent Integration

Extend coding mode profiles with control-flow selection.
react
pipeline
later architect
Update CodingAgent delegate resolution so a mode can map to either ReActAgent, ArchitectAgent, or PipelineAgent.
Add sensible defaults:
code: pipeline for structured code-fix tasks
debug: react by default
ask: react
docs: pipeline
architect: planner/pipeline hybrid or existing architect initially
Allow manifest override later if desired.
Files

agents/coding_agent.go
agents/modes.go
possibly framework/core/agent_spec.go if control-flow becomes manifest-configurable
Unit Tests

mode chooses pipeline delegate
mode chooses react delegate
BuildGraph respects control flow
nil-state handling still works
final output propagation still works
Phase 6: First Concrete Pipeline For Coding

Start with one narrow but valuable workflow for code-modification tasks.

Recommended V1 stages

ExploreStage
output: relevant files, tool suggestions
AnalyzeStage
output: typed issue list
PlanStage
output: typed fix plan
CodeStage
output: typed edit actions or patch description
VerifyStage
output: typed verification result
For small-model robustness, keep stages narrow.

Suggested typed outputs

FileSelection
IssueList
FixPlan
EditPlan
VerificationReport
Files

agents/stages/explore_stage.go
agents/stages/analyze_stage.go
agents/stages/plan_stage.go
agents/stages/code_stage.go
agents/stages/verify_stage.go
Unit Tests

each stage decode/validate
malformed JSON rejection
missing required fields rejection
valid outputs applied to context correctly
Phase 7: Tooling and Prompt Assembly

Reuse existing capability-policy and context-loading logic rather than rewriting it.
Add prompt helpers for stage-specific prompts.
Keep tool availability explicit per stage.
explore: read/search/ast/lsp
code: write/edit
verify: build/test/lint/read
Files

agents/stages/prompt_helpers.go
possibly refactor common logic out of agents/pattern/react.go
Unit Tests

stage tool filtering
prompt context inclusion/exclusion
correct tool scope by stage
Phase 8: Testsuite Coverage

Add pipeline-specific suites and extend dev-agent agenttest coverage.

New testsuite targets

coding pipeline Rust bug fix
coding pipeline multi-file fix
coding pipeline verify-only no edits
coding debug stays on ReAct if configured
malformed stage output fails cleanly
resume after partial pipeline execution
Files

testsuite/agenttests/coding.pipeline.rust.testsuite.yaml
testsuite/agenttests/coding.pipeline.go.testsuite.yaml
targeted unit tests in agents/ and framework/pipeline/
Migration Order

Add framework/pipeline contracts and runner.
Add persistence support for stage results.
Add PipelineAgent.
Add one coding pipeline workflow.
Wire CodingAgent mode selection to pipeline.
Add testsuites and run live Ollama validation.
Only after pipeline is stable, decide whether to migrate parts of ArchitectAgent or refactor shared logic out of ReAct.
Key Non-Goals For V1

Don’t replace ReAct.
Don’t force all agents onto pipeline.
Don’t add complex branching in pipeline v1.
Don’t over-generalize stage generics if any + strict validation gets you there faster.
Concrete Deliverables

framework/pipeline/*
persistence extension for stage results
agents/pipeline_agent.go
first coding pipeline stages
CodingAgent delegate selection update
unit tests across framework, agent, and testsuite layers
