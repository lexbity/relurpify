// Package buildtags documents the test tier conventions used across Relurpify.
//
// Tier 0 (default): no build tag, run with:
//
//	go test ./...
//
// Tier 1 (integration): files tagged with:
//
//	//go:build integration
//
// Requires real SQLite and filesystem access. Run with:
//
//	go test ./... -tags integration
//
// Tier 2 (scenario): files tagged with:
//
//	//go:build scenario
//
// Requires scripted LLM turns and must not depend on Ollama. Run with:
//
//	go test ./... -tags scenario
//
// Tier 3 (e2e): the existing testsuite/ coverage exercised in CI.
package buildtags
