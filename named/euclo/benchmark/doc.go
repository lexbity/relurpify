// Package benchmark contains deterministic Go benchmarks for Euclo's local
// engineering surfaces. The suite is intentionally non-LLM-dependent: it uses
// the same stub-model and temp-backed store patterns as Euclo's test harness so
// performance audits can focus on orchestration, persistence, projection, and
// relurpic-capability-owner overhead without live model variance.
//
// The benchmarks are organized by concern:
//   - agent_bench_test.go: top-level Agent.Execute overhead
//   - runtime_bench_test.go: hot classification, mode/profile, and context-expansion paths
//   - projection_bench_test.go: workflow read-model rebuild and subscription costs
//   - workload_bench_test.go: capability-owner workload scenarios
//
// Run with:
//
//	go test ./named/euclo/benchmark -run '^$' -bench .
package benchmark
