// Package benchmark contains deterministic Go benchmarks for euclo and
// archaeo-server. The suite is intentionally non-LLM-dependent: it uses the
// same stub-model and temp-backed store patterns as euclo's test harness so
// performance audits can focus on orchestration, persistence, and projection
// costs without live model variance.
//
// The benchmarks are organized by concern:
//   - agent_bench_test.go: top-level Agent.Execute overhead
//   - runtime_bench_test.go: hot classification, mode/profile, and context-expansion paths
//   - archaeo_bench_test.go: learning, archaeology, plan-versioning, and tension services
//   - projection_bench_test.go: workflow read-model rebuild and subscription costs
//
// Run with:
//
//	go test ./named/euclo/benchmark -run '^$' -bench .
package benchmark
