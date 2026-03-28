# Euclo Benchmarks

This package provides deterministic performance benchmarks for `named/euclo`
and `archaeo`.

It is intentionally non-LLM-dependent:
- uses `testutil/euclotestutil.StubModel`
- uses temp-backed SQLite workflow / plan / pattern stores
- uses local git fixtures where euclo expects checkpoint lookup

## Goals

- measure orchestration cost without live-model variance
- separate agent, archaeology, and projection surfaces
- make performance audits reproducible in CI and locally

## Benchmark Groups

- `BenchmarkAgentExecute`
  - top-level euclo execution overhead
  - includes simple, living-plan, learning-queue, and projection-heavy paths

- `BenchmarkClassifyTaskScored`
- `BenchmarkResolveModeAndProfile`
- `BenchmarkExpandContext`
  - runtime hot paths

- `BenchmarkArchaeologyPrepareLivingPlan`
- `BenchmarkLearningSyncPatternProposals`
- `BenchmarkLearningSyncAnchorDrifts`
- `BenchmarkLearningSyncTensions`
- `BenchmarkLearningResolve`
- `BenchmarkComparePlanVersions`
- `BenchmarkSyncActiveVersionWithExploration`
- `BenchmarkTensionSummaryByWorkflow`
  - archaeology / plan / learning service costs

- `BenchmarkWorkflowProjection`
- `BenchmarkDedicatedProjections`
- `BenchmarkWorkflowProjectionSubscription`
  - read-model rebuild and subscription polling costs

## Scale Presets

The suite uses flat sub-benchmarks:

- `small`
  - 10 timeline events
  - 5 learning interactions
  - 1 plan version

- `medium`
  - 100 timeline events
  - 25 learning interactions
  - 3 plan versions

- `large`
  - 1000 timeline events
  - 100 learning interactions
  - 10 plan versions

## Running

Run the full suite:

```bash
go test ./named/euclo/benchmark -run '^$' -bench .
```

Run one surface:

```bash
go test ./named/euclo/benchmark -run '^$' -bench 'BenchmarkWorkflowProjection'
```

Run with a longer sampling window:

```bash
go test ./named/euclo/benchmark -run '^$' -bench . -benchtime=3s
```

Show allocations clearly:

```bash
go test ./named/euclo/benchmark -run '^$' -bench . -benchmem
```

Persist results for audit comparison:

```bash
go test ./named/euclo/benchmark -run '^$' -bench . -benchmem | tee euclo-bench.txt
```

## Reading Results

- `ns/op`: end-to-end cost per operation
- `B/op`: bytes allocated per operation
- `allocs/op`: allocation pressure

Use the service-level benchmarks first to localize regressions, then confirm
them at the top-level `Agent.Execute` layer.

## Notes

- Projection subscription benchmarks measure the current polling-based design.
  Do not interpret them as RPC latency.
- These benchmarks are for euclo and archaeo-server internals, not relurpish UI
  rendering or live-model behavior.
